package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func (a *App) authOK(r *http.Request) bool {
	if tok := bearerToken(r.Header.Get("Authorization")); tok != "" && tok == a.config.ProxyAPIKey {
		return true
	}
	return strings.TrimSpace(r.Header.Get("x-api-key")) == a.config.ProxyAPIKey
}

func bearerToken(v string) string {
	parts := strings.Fields(v)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func apiStyleForRequest(r *http.Request) APIStyle {
	path := strings.TrimPrefix(r.URL.Path, "/v1")
	if strings.HasPrefix(path, "/messages") || strings.HasPrefix(path, "/complete") {
		return APIStyleAnthropic
	}
	for name := range r.Header {
		if strings.HasPrefix(strings.ToLower(name), "anthropic-") {
			return APIStyleAnthropic
		}
	}
	return APIStyleOpenAI
}

func (a *App) proxyV1(w http.ResponseWriter, r *http.Request, style APIStyle) {
	start := time.Now()
	r.Body = http.MaxBytesReader(w, r.Body, a.config.MaxRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}
	_ = r.Body.Close()
	orig := r.Context()
	for attempts := 0; attempts < len(a.config.UpstreamAPIKeys); attempts++ {
		idx, key, ok := a.keys.Current()
		if !ok {
			break
		}
		resp, reqErr := a.doUpstream(orig, r, body, key, style)
		if reqErr != nil {
			a.recordRequest(r.Method, r.URL.Path, idx, http.StatusBadGateway, time.Since(start))
			http.Error(w, reqErr.Error(), http.StatusBadGateway)
			return
		}
		if resp.StatusCode == http.StatusTooManyRequests && isQuota429(resp) {
			_ = resp.Body.Close()
			a.keys.MarkExhausted(idx)
			a.recordRequest(r.Method, r.URL.Path, idx, 429, time.Since(start))
			if a.keys.ShouldNotifySwitch(idx) {
				a.sender.NotifySwitch(idx, a.keys.Status())
			}
			if a.keys.ShouldNotifyAllExhausted() {
				a.sender.NotifyAllExhausted(a.keys.Status())
				writeAPIError(w, style, 429, "rate_limit_exceeded", "all upstream keys exhausted")
				return
			}
			continue
		}
		a.recordRequest(r.Method, r.URL.Path, idx, resp.StatusCode, time.Since(start))
		copyResponse(w, resp)
		return
	}
	if a.keys.AllExhausted() {
		if a.keys.ShouldNotifyAllExhausted() {
			a.sender.NotifyAllExhausted(a.keys.Status())
		}
		a.recordRequest(r.Method, r.URL.Path, -1, 429, time.Since(start))
		writeAPIError(w, style, 429, "rate_limit_exceeded", "all upstream keys exhausted")
		return
	}
	a.recordRequest(r.Method, r.URL.Path, -1, http.StatusBadGateway, time.Since(start))
	writeAPIError(w, style, 502, "bad_gateway", "upstream unavailable")
}

func (a *App) recordRequest(method, path string, keyIndex, status int, dur time.Duration) {
	if a.requestLog == nil {
		return
	}
	a.requestLog.Add(RequestLogEntry{
		Timestamp:  time.Now().UTC(),
		Method:     method,
		Path:       path,
		KeyIndex:   keyIndex,
		Status:     status,
		DurationMs: dur.Milliseconds(),
	})
}

func (a *App) doUpstream(ctx context.Context, r *http.Request, body []byte, key string, apiStyle APIStyle) (*http.Response, error) {
	path := strings.TrimPrefix(r.URL.EscapedPath(), "/v1")
	if path == "" {
		path = "/"
	}
	u := a.config.UpstreamBaseURL + path
	if r.URL.RawQuery != "" {
		u += "?" + r.URL.RawQuery
	}
	req, err := http.NewRequestWithContext(ctx, r.Method, u, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	copyHeaders(req.Header, r.Header)
	if apiStyle == APIStyleAnthropic {
		req.Header.Set("x-api-key", key)
		if strings.TrimSpace(req.Header.Get("anthropic-version")) == "" {
			req.Header.Set("anthropic-version", "2023-06-01")
		}
		if strings.TrimSpace(req.Header.Get("User-Agent")) == "" {
			req.Header.Set("User-Agent", "anthropic-sdk-go/1.0.0")
		}
	} else {
		req.Header.Set("Authorization", "Bearer "+key)
		if strings.TrimSpace(req.Header.Get("User-Agent")) == "" {
			req.Header.Set("User-Agent", "OpenAI/Python 1.0.0")
		}
	}
	stripHopByHopHeaders(req.Header)
	return a.client.Do(req)
}

func copyHeaders(dst, src http.Header) {
	for k, v := range src {
		if strings.EqualFold(k, "Authorization") || strings.EqualFold(k, "x-api-key") {
			continue
		}
		for _, s := range v {
			dst.Add(k, s)
		}
	}
}

func stripHopByHopHeaders(h http.Header) {
	hop := map[string]struct{}{
		"Connection":          {},
		"Proxy-Authorization": {},
		"Proxy-Authenticate":  {},
		"Keep-Alive":          {},
		"Te":                  {},
		"Trailer":             {},
		"Transfer-Encoding":   {},
		"Upgrade":             {},
	}
	for _, v := range h.Values("Connection") {
		for _, part := range strings.Split(v, ",") {
			if name := strings.TrimSpace(part); name != "" {
				hop[http.CanonicalHeaderKey(name)] = struct{}{}
			}
		}
	}
	for k := range hop {
		h.Del(k)
	}
}

func copyResponse(w http.ResponseWriter, resp *http.Response) {
	defer resp.Body.Close()
	stripHopByHopHeaders(resp.Header)
	for k, v := range resp.Header {
		for _, s := range v {
			w.Header().Add(k, s)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func isQuota429(resp *http.Response) bool {
	if resp.StatusCode != 429 {
		return false
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	var m map[string]any
	if json.Unmarshal(body, &m) == nil {
		if e, ok := m["error"].(map[string]any); ok {
			if code, _ := e["code"].(string); code == "insufficient_quota" || code == "usage_not_included" {
				return true
			}
			typ, _ := e["type"].(string)
			msg, _ := e["message"].(string)
			lowType := strings.ToLower(typ)
			lowMsg := strings.ToLower(msg)
			if strings.Contains(lowType, "usage") || strings.Contains(lowType, "quota") || strings.Contains(lowType, "freeusagelimit") {
				return true
			}
			if strings.Contains(lowMsg, "quota") || strings.Contains(lowMsg, "exhausted") || strings.Contains(lowMsg, "usage limit") || strings.Contains(lowMsg, "credit balance") || strings.Contains(lowMsg, "billing limit") {
				return true
			}
		}
	}
	return strings.Contains(strings.ToLower(resp.Header.Get("X-RateLimit-Reason")), "quota")
}

func writeAPIError(w http.ResponseWriter, style APIStyle, status int, code, message string) {
	if style == APIStyleAnthropic {
		writeAnthropicError(w, status, code, message)
		return
	}
	writeOpenAIError(w, status, code, message)
}

func writeOpenAIError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    "invalid_request_error",
			"param":   nil,
			"code":    code,
		},
	})
}

func writeAnthropicError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    anthropicErrorType(status, code),
			"message": message,
		},
	})
}

func anthropicErrorType(status int, code string) string {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return "authentication_error"
	case status == http.StatusTooManyRequests || code == "rate_limit_exceeded":
		return "rate_limit_error"
	case status >= 500:
		return "api_error"
	default:
		return "invalid_request_error"
	}
}

func (a *App) handleReadyz(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{"ready": false}
	if err := a.config.Validate(); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, resp)
		return
	}
	_, key, ok := a.keys.Current()
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, resp)
		return
	}
	if err := a.checkUpstreamReady(r.Context(), key); err != nil {
		resp["error"] = "upstream not ready"
		writeJSON(w, http.StatusServiceUnavailable, resp)
		return
	}
	resp["ready"] = true
	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (a *App) checkUpstreamReady(ctx context.Context, key string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(a.config.UpstreamBaseURL, "/")+"/models", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("User-Agent", "OpenAI/Python 1.0.0")
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("upstream status %d", resp.StatusCode)
}

func (a *App) handleValidateKeys(w http.ResponseWriter, r *http.Request) {
	results := make([]ValidateKeyResult, 0, len(a.config.UpstreamAPIKeys))
	for i, key := range a.config.UpstreamAPIKeys {
		res := ValidateKeyResult{Index: i}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(a.config.UpstreamBaseURL, "/")+"/models", nil)
		if err != nil {
			cancel()
			res.State = string(KeyUnknown)
			res.Status = http.StatusBadGateway
			res.Error = err.Error()
			results = append(results, res)
			continue
		}
		req.Header.Set("Authorization", "Bearer "+key.Key)
		req.Header.Set("User-Agent", "OpenAI/Python 1.0.0")
		resp, err := a.client.Do(req)
		cancel()
		if err != nil {
			res.State = string(KeyUnknown)
			res.Status = http.StatusBadGateway
			res.Error = err.Error()
			results = append(results, res)
			continue
		}
		res.Status = resp.StatusCode
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			a.keys.MarkAvailable(i)
			res.State = string(KeyAvailable)
		} else if resp.StatusCode == http.StatusTooManyRequests && isQuota429(resp) {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			a.keys.SetState(i, KeyExhausted)
			res.State = string(KeyExhausted)
			res.Error = "quota exhausted"
		} else {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			a.keys.SetState(i, KeyUnknown)
			res.State = string(KeyUnknown)
			res.Error = fmt.Sprintf("status %d", resp.StatusCode)
		}
		results = append(results, res)
	}
	writeJSON(w, http.StatusOK, ValidateKeysResponse{Results: results})
}
