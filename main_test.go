package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestKeyManagerNoCurrentWhenExhausted(t *testing.T) {
	km := NewKeyManager([]string{"a"}, nil)
	km.MarkExhausted(0)
	if _, _, ok := km.Current(); ok {
		t.Fatal("expected no current key")
	}
}

func TestBearerTokenCaseInsensitive(t *testing.T) {
	if got := bearerToken("bearer abc"); got != "abc" {
		t.Fatalf("got %q", got)
	}
	if got := bearerToken("BEARER abc"); got != "abc" {
		t.Fatalf("got %q", got)
	}
}

func TestIsRateLimited(t *testing.T) {
	if !isRateLimited(&http.Response{StatusCode: 429}) {
		t.Fatal("expected 429 to be rate limited")
	}
	if isRateLimited(&http.Response{StatusCode: 200}) {
		t.Fatal("expected 200 not to be rate limited")
	}
	if isRateLimited(nil) {
		t.Fatal("expected nil response not to be rate limited")
	}
}

func TestRequestTooLargeReturns413(t *testing.T) {
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []KeyEntry{{Key: "u"}}, MaxRequestBodyBytes: 4})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte("12345")))
	req.Header.Set("Authorization", "Bearer p")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestRootOpenAIPathProxies(t *testing.T) {
	var gotPath, gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []KeyEntry{{Key: "u"}}, UpstreamBaseURL: upstream.URL, MaxRequestBodyBytes: 1024})
	req := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(`{"model":"glm-5.1","messages":[]}`))
	req.Header.Set("Authorization", "Bearer p")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d body %s", rec.Code, rec.Body.String())
	}
	if gotPath != "/chat/completions" || gotAuth != "Bearer u" {
		t.Fatalf("unexpected upstream path=%q auth=%q", gotPath, gotAuth)
	}
}

func TestDoUpstreamSetsDefaultUserAgent(t *testing.T) {
	var gotUA string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []KeyEntry{{Key: "u"}}, UpstreamBaseURL: upstream.URL})
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	resp, err := app.doUpstream(req.Context(), req, nil, "u", APIStyleOpenAI)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if gotUA != "OpenAI/Python 1.0.0" {
		t.Fatalf("got user-agent %q", gotUA)
	}
}

func TestDoUpstreamAnthropicSetsHeaders(t *testing.T) {
	var gotPath, gotKey, gotAuth, gotVersion, gotUA string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("x-api-key")
		gotAuth = r.Header.Get("Authorization")
		gotVersion = r.Header.Get("anthropic-version")
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []KeyEntry{{Key: "u"}}, UpstreamBaseURL: upstream.URL})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"minimax-m3","messages":[]}`))
	req.Header.Set("x-api-key", "p")
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := app.doUpstream(req.Context(), req, []byte(`{"model":"minimax-m3","messages":[]}`), "u", APIStyleAnthropic)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if gotPath != "/messages" || gotKey != "u" || gotAuth != "" || gotVersion != "2023-06-01" || gotUA != "anthropic-sdk-go/1.0.0" {
		t.Fatalf("unexpected upstream request path=%q key=%q auth=%q version=%q ua=%q", gotPath, gotKey, gotAuth, gotVersion, gotUA)
	}
}

func TestProxyAnthropicMessagesCyclesKeys(t *testing.T) {
	var gotKeys []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			http.NotFound(w, r)
			return
		}
		gotKeys = append(gotKeys, r.Header.Get("x-api-key"))
		if r.Header.Get("x-api-key") == "bad" {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"usage limit exhausted"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","content":[]}`))
	}))
	defer upstream.Close()

	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []KeyEntry{{Key: "bad"}, {Key: "good"}}, UpstreamBaseURL: upstream.URL, MaxRequestBodyBytes: 1024})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"minimax-m3","messages":[]}`))
	req.Header.Set("x-api-key", "p")
	req.Header.Set("anthropic-version", "2023-06-01")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d body %s", rec.Code, rec.Body.String())
	}
	if strings.Join(gotKeys, ",") != "bad,good" {
		t.Fatalf("unexpected keys: %v", gotKeys)
	}
}

func TestAuthFailureReturnsOpenAIJSON(t *testing.T) {
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []KeyEntry{{Key: "u"}}, UpstreamBaseURL: "http://example.com", MaxRequestBodyBytes: 1})
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("ct %q", ct)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["error"] == nil {
		t.Fatal("missing error object")
	}
}

func TestAuthFailureReturnsAnthropicJSON(t *testing.T) {
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []KeyEntry{{Key: "u"}}, UpstreamBaseURL: "http://example.com", MaxRequestBodyBytes: 1})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	req.Header.Set("anthropic-version", "2023-06-01")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	errObj, ok := payload["error"].(map[string]any)
	if payload["type"] != "error" || !ok || errObj["type"] != "authentication_error" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestRootAnthropicAuthFailureReturnsAnthropicJSON(t *testing.T) {
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []KeyEntry{{Key: "u"}}, UpstreamBaseURL: "http://example.com", MaxRequestBodyBytes: 1})
	req := httptest.NewRequest(http.MethodPost, "/messages", nil)
	req.Header.Set("anthropic-version", "2023-06-01")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	errObj, ok := payload["error"].(map[string]any)
	if payload["type"] != "error" || !ok || errObj["type"] != "authentication_error" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestValidateKeysEndpoint(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("User-Agent") != "OpenAI/Python 1.0.0" {
			t.Fatalf("unexpected user-agent %q", r.Header.Get("User-Agent"))
		}
		if r.Header.Get("Authorization") == "Bearer good" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[]}`))
			return
		}
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"quota exceeded","code":"insufficient_quota"}}`))
	}))
	defer upstream.Close()
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []KeyEntry{{Key: "good"}, {Key: "bad"}}, UpstreamBaseURL: upstream.URL, MaxRequestBodyBytes: 1})
	req := httptest.NewRequest(http.MethodPost, "/admin/validate-keys", nil)
	req.Header.Set("Authorization", "Bearer p")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
	var out ValidateKeysResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != 2 {
		t.Fatalf("got %d results", len(out.Results))
	}
	if out.Results[0].State != string(KeyAvailable) || out.Results[1].State != string(KeyExhausted) {
		t.Fatalf("unexpected results: %+v", out.Results)
	}
}

func TestReadyzUnauthenticated(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []KeyEntry{{Key: "good"}}, UpstreamBaseURL: upstream.URL, MaxRequestBodyBytes: 1})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
}


