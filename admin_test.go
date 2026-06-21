package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdminGetSettings(t *testing.T) {
	app := newApp(Config{
		ProxyAPIKey:         "p",
		UpstreamAPIKeys:     []KeyEntry{{Key: "u"}},
		UpstreamBaseURL:     "http://example.com",
		MaxRequestBodyBytes: 1,
		RequestLogSize:      500,
	})
	app.requestLog = NewRequestLog(500)
	req := httptest.NewRequest(http.MethodGet, "/admin/settings", nil)
	req.Header.Set("Authorization", "Bearer p")
	rec := httptest.NewRecorder()
	setupMux(app).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	// no explicit listen_addr set in test Config, expect empty string
	if body["listen_addr"] != "" {
		t.Fatalf("unexpected listen_addr: %v", body["listen_addr"])
	}
}

func TestAdminAddKey(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			if r.Header.Get("Authorization") == "Bearer valid-key" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"data":[]}`))
				return
			}
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"quota exceeded","code":"insufficient_quota"}}`))
		}
	}))
	defer upstream.Close()
	app := newApp(Config{
		ProxyAPIKey:         "p",
		UpstreamAPIKeys:     []KeyEntry{{Key: "existing"}},
		UpstreamBaseURL:     upstream.URL,
		MaxRequestBodyBytes: 1,
	})
	mux := setupMux(app)
	req := httptest.NewRequest(http.MethodPost, "/admin/keys",
		strings.NewReader(`{"key":"valid-key"}`))
	req.Header.Set("Authorization", "Bearer p")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["index"] != float64(1) {
		t.Fatalf("expected index 1, got %v", resp["index"])
	}
	if resp["valid"] != true {
		t.Fatalf("expected valid=true")
	}
}

func TestAdminAddInvalidKey(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"quota exceeded","code":"insufficient_quota"}}`))
	}))
	defer upstream.Close()
	app := newApp(Config{
		ProxyAPIKey:         "p",
		UpstreamAPIKeys:     []KeyEntry{{Key: "existing"}},
		UpstreamBaseURL:     upstream.URL,
		MaxRequestBodyBytes: 1,
	})
	mux := setupMux(app)
	req := httptest.NewRequest(http.MethodPost, "/admin/keys",
		strings.NewReader(`{"key":"bad-key"}`))
	req.Header.Set("Authorization", "Bearer p")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["valid"] != false {
		t.Fatalf("expected valid=false for exhausted key")
	}
}

func TestAdminDeleteKey(t *testing.T) {
	app := newApp(Config{
		ProxyAPIKey:         "p",
		UpstreamAPIKeys:     []KeyEntry{{Key: "a"}, {Key: "b"}},
		UpstreamBaseURL:     "http://example.com",
		MaxRequestBodyBytes: 1,
	})
	mux := setupMux(app)
	req := httptest.NewRequest(http.MethodDelete, "/admin/keys/1", nil)
	req.Header.Set("Authorization", "Bearer p")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	if app.keys.Status().CurrentKeyIndex != 0 {
		t.Fatalf("expected current 0 after deleting key 1")
	}
}

func TestAdminDeleteLastKeyReturns409(t *testing.T) {
	app := newApp(Config{
		ProxyAPIKey:         "p",
		UpstreamAPIKeys:     []KeyEntry{{Key: "only"}},
		UpstreamBaseURL:     "http://example.com",
		MaxRequestBodyBytes: 1,
	})
	mux := setupMux(app)
	req := httptest.NewRequest(http.MethodDelete, "/admin/keys/0", nil)
	req.Header.Set("Authorization", "Bearer p")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

func TestAdminReorderKeys(t *testing.T) {
	app := newApp(Config{
		ProxyAPIKey:         "p",
		UpstreamAPIKeys:     []KeyEntry{{Key: "a"}, {Key: "b"}, {Key: "c"}},
		UpstreamBaseURL:     "http://example.com",
		MaxRequestBodyBytes: 1,
	})
	mux := setupMux(app)
	req := httptest.NewRequest(http.MethodPut, "/admin/keys/reorder",
		strings.NewReader(`{"indices":[2,0,1]}`))
	req.Header.Set("Authorization", "Bearer p")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	if app.keys.Status().CurrentKeyIndex != 1 {
		t.Fatalf("expected current 1 after reorder, got %d", app.keys.Status().CurrentKeyIndex)
	}
}

func TestAdminRequests(t *testing.T) {
	app := newApp(Config{
		ProxyAPIKey:         "p",
		UpstreamAPIKeys:     []KeyEntry{{Key: "u"}},
		UpstreamBaseURL:     "http://example.com",
		MaxRequestBodyBytes: 1,
		RequestLogSize:      100,
	})
	app.requestLog = NewRequestLog(100)
	app.requestLog.Add(RequestLogEntry{Method: "GET", Path: "/v1/models", KeyIndex: 0, Status: 200, DurationMs: 10})
	app.requestLog.Add(RequestLogEntry{Method: "POST", Path: "/v1/chat/completions", KeyIndex: 0, Status: 200, DurationMs: 42})
	mux := setupMux(app)
	req := httptest.NewRequest(http.MethodGet, "/admin/requests", nil)
	req.Header.Set("Authorization", "Bearer p")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
	var body map[string]any
	json.Unmarshal(rec.Body.Bytes(), &body)
	entries := body["entries"].([]any)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	total := body["total"].(float64)
	if total != 2 {
		t.Fatalf("expected total 2, got %f", total)
	}
}

func TestAdminAddKeyPersistsToConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	t.Setenv("SWITCHBOARD_GO_CONFIG", configPath)
	app := newApp(Config{
		ProxyAPIKey:         "p",
		UpstreamAPIKeys:     []KeyEntry{{Key: "existing"}},
		UpstreamBaseURL:     "http://example.com",
		MaxRequestBodyBytes: 1,
	})
	mux := setupMux(app)
	req := httptest.NewRequest(http.MethodPost, "/admin/keys",
		strings.NewReader(`{"key":"new-key","name":"my key"}`))
	req.Header.Set("Authorization", "Bearer p")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfigFromBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.UpstreamAPIKeys) != 2 {
		t.Fatalf("expected 2 keys in config, got %d", len(cfg.UpstreamAPIKeys))
	}
	if cfg.UpstreamAPIKeys[1].Key != "new-key" || cfg.UpstreamAPIKeys[1].Name != "my key" {
		t.Fatalf("unexpected key entry: %+v", cfg.UpstreamAPIKeys[1])
	}
}

func TestAdminDeleteKeyPersistsToConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	t.Setenv("SWITCHBOARD_GO_CONFIG", configPath)
	app := newApp(Config{
		ProxyAPIKey:         "p",
		UpstreamAPIKeys:     []KeyEntry{{Key: "a"}, {Key: "b"}},
		UpstreamBaseURL:     "http://example.com",
		MaxRequestBodyBytes: 1,
	})
	mux := setupMux(app)
	req := httptest.NewRequest(http.MethodDelete, "/admin/keys/0", nil)
	req.Header.Set("Authorization", "Bearer p")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfigFromBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.UpstreamAPIKeys) != 1 {
		t.Fatalf("expected 1 key in config, got %d", len(cfg.UpstreamAPIKeys))
	}
	if cfg.UpstreamAPIKeys[0].Key != "b" {
		t.Fatalf("expected key 'b', got %+v", cfg.UpstreamAPIKeys[0])
	}
}

func TestAdminReorderPersistsToConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	t.Setenv("SWITCHBOARD_GO_CONFIG", configPath)
	app := newApp(Config{
		ProxyAPIKey:         "p",
		UpstreamAPIKeys:     []KeyEntry{{Key: "a"}, {Key: "b"}, {Key: "c"}},
		UpstreamBaseURL:     "http://example.com",
		MaxRequestBodyBytes: 1,
	})
	mux := setupMux(app)
	req := httptest.NewRequest(http.MethodPut, "/admin/keys/reorder",
		strings.NewReader(`{"indices":[2,0,1]}`))
	req.Header.Set("Authorization", "Bearer p")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfigFromBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.UpstreamAPIKeys) != 3 {
		t.Fatalf("expected 3 keys in config, got %d", len(cfg.UpstreamAPIKeys))
	}
	if cfg.UpstreamAPIKeys[0].Key != "c" || cfg.UpstreamAPIKeys[1].Key != "a" || cfg.UpstreamAPIKeys[2].Key != "b" {
		t.Fatalf("unexpected order: %+v", cfg.UpstreamAPIKeys)
	}
}
