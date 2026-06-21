package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type KeyInput struct {
	Key  string `json:"key"`
	Name string `json:"name,omitempty"`
}

type AddKeyResponse struct {
	Index int            `json:"index"`
	State string         `json:"state"`
	Valid bool           `json:"valid"`
	Error string         `json:"error,omitempty"`
	Keys  []PerKeyStatus `json:"keys"`
}

type DeleteKeyResponse struct {
	CurrentKeyIndex int            `json:"current_key_index"`
	Keys            []PerKeyStatus `json:"keys"`
}

type ReorderInput struct {
	Indices []int `json:"indices"`
}

type SettingsInput map[string]any

func adminAuth(a *App, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.authOK(r) {
			writeAPIError(w, apiStyleForRequest(r), http.StatusUnauthorized, "invalid_api_key", "Unauthorized")
			return
		}
		next(w, r)
	}
}

func setupMux(a *App) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /admin/status", adminAuth(a, a.handleStatus))
	mux.HandleFunc("GET /admin/keys", adminAuth(a, a.handleListKeys))
	mux.HandleFunc("POST /admin/keys", adminAuth(a, a.handleAddKey))
	mux.HandleFunc("DELETE /admin/keys/{index}", adminAuth(a, a.handleDeleteKey))
	mux.HandleFunc("PUT /admin/keys/reorder", adminAuth(a, a.handleReorderKeys))
	mux.HandleFunc("GET /admin/settings", adminAuth(a, a.handleGetSettings))
	mux.HandleFunc("PUT /admin/settings", adminAuth(a, a.handleUpdateSettings))
	mux.HandleFunc("GET /admin/requests", adminAuth(a, a.handleGetRequests))
	mux.HandleFunc("POST /admin/validate-key", adminAuth(a, a.handleValidateSingleKey))
	mux.HandleFunc("POST /admin/validate-keys", adminAuth(a, a.handleValidateKeys))
	mux.HandleFunc("GET /admin/update-check", adminAuth(a, a.handleUpdateCheck))
	mux.HandleFunc("POST /admin/update", adminAuth(a, a.handleUpdate))

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /readyz", a.handleReadyz)

	for _, method := range []string{"GET", "POST", "PUT", "DELETE", "PATCH"} {
		mux.HandleFunc(method+" /v1/{path...}", a.proxyHandler)
		mux.HandleFunc(method+" /{path...}", a.proxyHandler)
	}

	if dashFS, err := getDashboardFS(); err == nil {
		mux.Handle("GET /dashboard/{path...}", http.StripPrefix("/dashboard/",
			http.FileServer(http.FS(dashFS))))
	}

	return mux
}

func (a *App) saveKeyConfig(entries ...[]KeyEntry) error {
	a.configMu.Lock()
	defer a.configMu.Unlock()
	if len(entries) > 0 {
		a.config.UpstreamAPIKeys = entries[0]
	} else {
		a.config.UpstreamAPIKeys = a.keys.KeyEntries()
	}
	path := ConfigPath()
	if path == "" {
		return nil
	}
	return SaveConfig(a.config, path)
}

func (a *App) proxyHandler(w http.ResponseWriter, r *http.Request) {
	if !a.authOK(r) {
		writeAPIError(w, apiStyleForRequest(r), http.StatusUnauthorized, "invalid_api_key", "Unauthorized")
		return
	}
	a.proxyV1(w, r, apiStyleForRequest(r))
}

func (a *App) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(a.keys.Status())
}

func (a *App) handleListKeys(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(a.keys.Status())
}

func (a *App) handleAddKey(w http.ResponseWriter, r *http.Request) {
	var input KeyInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if strings.TrimSpace(input.Key) == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request", "key is required")
		return
	}
	state, valid, errStr, transportErr := a.validateSingleKey(input.Key)
	if transportErr {
		writeOpenAIError(w, http.StatusBadGateway, "validation_failed", errStr)
		return
	}
	newEntries := append(a.keys.KeyEntries(), KeyEntry{Key: input.Key, Name: input.Name})
	if err := a.saveKeyConfig(newEntries); err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	idx := a.keys.AddKey(input.Key, input.Name)
	if !valid && state != string(KeyExhausted) {
		a.keys.SetState(idx, KeyUnknown)
	}

	isValid := valid || state == string(KeyExhausted)
	resp := AddKeyResponse{
		Index: idx,
		State: state,
		Valid: isValid,
		Error: errStr,
		Keys:  a.keys.Status().Keys,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (a *App) handleDeleteKey(w http.ResponseWriter, r *http.Request) {
	idxStr := r.PathValue("index")
	idx, err := strconv.Atoi(idxStr)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request", "invalid key index")
		return
	}
	entries := a.keys.KeyEntries()
	if idx < 0 || idx >= len(entries) {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request", "invalid key index")
		return
	}
	if len(entries) == 1 {
		writeOpenAIError(w, http.StatusConflict, "cannot_remove", "cannot remove last key")
		return
	}
	newEntries := append(entries[:idx], entries[idx+1:]...)
	if err := a.saveKeyConfig(newEntries); err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	if err := a.keys.RemoveKey(idx); err != nil {
		writeOpenAIError(w, http.StatusConflict, "cannot_remove", err.Error())
		return
	}
	resp := DeleteKeyResponse{
		CurrentKeyIndex: a.keys.Status().CurrentKeyIndex,
		Keys:            a.keys.Status().Keys,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (a *App) handleReorderKeys(w http.ResponseWriter, r *http.Request) {
	var input ReorderInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	entries := a.keys.KeyEntries()
	if len(input.Indices) != len(entries) {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request", "permutation length mismatch")
		return
	}
	seen := map[int]bool{}
	for _, idx := range input.Indices {
		if idx < 0 || idx >= len(entries) || seen[idx] {
			writeOpenAIError(w, http.StatusBadRequest, "invalid_request", "invalid permutation")
			return
		}
		seen[idx] = true
	}
	newEntries := make([]KeyEntry, len(entries))
	for newIdx, oldIdx := range input.Indices {
		newEntries[newIdx] = entries[oldIdx]
	}
	if err := a.saveKeyConfig(newEntries); err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	if err := a.keys.Reorder(input.Indices); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(a.keys.Status())
}

func (a *App) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	cfg := a.config
	maskedPassword := ""
	if cfg.SMTPPassword != "" {
		maskedPassword = "******"
	}
	resp := map[string]any{
		"listen_addr":            cfg.ListenAddr,
		"upstream_base_url":      cfg.UpstreamBaseURL,
		"upstream_api_keys":      cfg.UpstreamAPIKeys,
		"max_request_body_bytes": cfg.MaxRequestBodyBytes,
		"request_log_size":       cfg.RequestLogSize,
		"smtp_host":              cfg.SMTPHost,
		"smtp_port":              cfg.SMTPPort,
		"smtp_username":          cfg.SMTPUsername,
		"smtp_password":          maskedPassword,
		"smtp_from":              cfg.SMTPFrom,
		"smtp_to":                cfg.SMTPTo,
		"smtp_tls":               cfg.SMTPTLS,
		"smtp_starttls":          cfg.SMTPStartTLS,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (a *App) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var input SettingsInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	a.configMu.Lock()
	defer a.configMu.Unlock()
	if v, ok := input["listen_addr"]; ok {
		if s, ok := v.(string); ok {
			a.config.ListenAddr = s
		}
	}
	if v, ok := input["upstream_base_url"]; ok {
		if s, ok := v.(string); ok {
			a.config.UpstreamBaseURL = strings.TrimRight(s, "/")
		}
	}
	if v, ok := input["max_request_body_bytes"]; ok {
		if f, ok := v.(float64); ok {
			a.config.MaxRequestBodyBytes = int64(f)
		}
	}
	if v, ok := input["request_log_size"]; ok {
		if f, ok := v.(float64); ok {
			a.config.RequestLogSize = int(f)
		}
	}
	if _, ok := input["smtp_password"]; ok {
		if s, ok := input["smtp_password"].(string); ok && s == "" {
			a.config.SMTPPassword = ""
		} else if s, ok := input["smtp_password"].(string); ok && s != "" && s != "******" {
			a.config.SMTPPassword = s
		}
	}
	if v, ok := input["smtp_host"]; ok && v != nil {
		if s, ok := v.(string); ok {
			a.config.SMTPHost = s
		}
	}
	if v, ok := input["smtp_port"]; ok {
		if f, ok := v.(float64); ok {
			a.config.SMTPPort = int(f)
		}
	}
	if v, ok := input["smtp_username"]; ok && v != nil {
		if s, ok := v.(string); ok {
			a.config.SMTPUsername = s
		}
	}
	if v, ok := input["smtp_from"]; ok && v != nil {
		if s, ok := v.(string); ok {
			a.config.SMTPFrom = s
		}
	}
	if v, ok := input["smtp_to"]; ok && v != nil {
		if s, ok := v.(string); ok {
			a.config.SMTPTo = s
		}
	}
	if v, ok := input["smtp_tls"]; ok {
		if b, ok := v.(bool); ok {
			a.config.SMTPTLS = b
		}
	}
	if v, ok := input["smtp_starttls"]; ok {
		if b, ok := v.(bool); ok {
			a.config.SMTPStartTLS = b
		}
	}
	if err := a.config.Validate(); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_settings", err.Error())
		return
	}
	path := ConfigPath()
	if path != "" {
		if err := SaveConfig(a.config, path); err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, "save_failed", err.Error())
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}

func (a *App) handleGetRequests(w http.ResponseWriter, r *http.Request) {
	if a.requestLog == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"entries": []RequestLogEntry{}, "total": 0})
		return
	}
	entries := a.requestLog.GetAll()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"entries": entries,
		"total":   len(entries),
	})
}

func (a *App) handleValidateSingleKey(w http.ResponseWriter, r *http.Request) {
	var input KeyInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	state, valid, errStr, _ := a.validateSingleKey(input.Key)
	isValid := valid || state == string(KeyExhausted)
	resp := map[string]any{"state": state, "valid": isValid}
	if errStr != "" {
		resp["error"] = errStr
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (a *App) validateSingleKey(key string) (state string, valid bool, errStr string, transportErr bool) {
	baseURL := strings.TrimRight(a.config.UpstreamBaseURL, "/")
	model, modelsState, modelsValid, modelsErr, modelsTransport := a.validateModels(key, baseURL+"/models")
	if modelsErr != "" {
		return modelsState, modelsValid, modelsErr, modelsTransport
	}
	if !modelsValid {
		return modelsState, false, "no models available", false
	}
	completionBody := fmt.Sprintf(`{"model":"%s","messages":[{"role":"user","content":"hi"}],"max_tokens":1}`, model)
	state, valid, errStr, transportErr = a.validateWithEndpoint(key, baseURL+"/chat/completions", "POST", []byte(completionBody))
	if !valid && errStr == "" {
		return string(KeyAvailable), true, "", false
	}
	return state, valid, errStr, transportErr
}

func (a *App) validateModels(key, url string) (model string, state string, valid bool, errStr string, transportErr bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", string(KeyUnknown), false, err.Error(), true
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("User-Agent", "OpenAI/Python 1.0.0")
	resp, err := a.client.Do(req)
	if err != nil {
		return "", string(KeyUnknown), false, err.Error(), true
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		// Parse response to extract a model ID
		var modelsResp struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if decodeErr := json.NewDecoder(resp.Body).Decode(&modelsResp); decodeErr == nil && len(modelsResp.Data) > 0 {
			return modelsResp.Data[0].ID, string(KeyAvailable), true, "", false
		}
		return "", string(KeyAvailable), true, "", false
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return "", string(KeyUnknown), false, fmt.Sprintf("status %d: invalid key", resp.StatusCode), false
	case resp.StatusCode == http.StatusTooManyRequests && isQuota429(resp):
		return "", string(KeyExhausted), false, "quota exhausted", false
	default:
		return "", string(KeyUnknown), false, fmt.Sprintf("status %d", resp.StatusCode), false
	}
}

func (a *App) validateWithEndpoint(key, url, method string, body []byte) (state string, valid bool, errStr string, transportErr bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return string(KeyUnknown), false, err.Error(), true
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("User-Agent", "OpenAI/Python 1.0.0")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return string(KeyUnknown), false, err.Error(), true
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return string(KeyAvailable), true, "", false
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return string(KeyUnknown), false, fmt.Sprintf("status %d: invalid key", resp.StatusCode), false
	case resp.StatusCode == http.StatusTooManyRequests && isQuota429(resp):
		return string(KeyExhausted), false, "quota exhausted", false
	default:
		return string(KeyUnknown), false, fmt.Sprintf("status %d", resp.StatusCode), false
	}
}

type releaseInfo struct {
	TagName string `json:"tag_name"`
}

var userAgent = "switchboard-go"

func (a *App) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.github.com/repos/kjhq/switchboard-go/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := a.client.Do(req)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"current": Version, "latest": "", "update_available": false,
			"error": "failed to check updates",
		})
		return
	}
	defer resp.Body.Close()

	var rel releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil || rel.TagName == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"current": Version, "latest": "", "update_available": false,
			"error": "failed to parse release info",
		})
		return
	}

	available := rel.TagName != Version && Version != "dev"
	writeJSON(w, http.StatusOK, map[string]any{
		"current":          Version,
		"latest":           rel.TagName,
		"update_available": available,
	})
}

func (a *App) handleUpdate(w http.ResponseWriter, r *http.Request) {
	composePath := a.config.DockerComposePath

	pull := exec.Command("docker", "pull", "ghcr.io/kjhq/switchboard-go:latest")
	if out, err := pull.CombinedOutput(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"status": "error", "error": "pull failed: " + string(out),
		})
		return
	}

	restart := exec.Command("docker", "compose", "-f", composePath, "up", "-d")
	if out, err := restart.CombinedOutput(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"status": "error", "error": "restart failed: " + string(out),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "updated",
		"version": "latest",
	})
}
