package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestKeyManagerNoCurrentWhenExhausted(t *testing.T) {
	km := NewKeyManager([]string{"a"})
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

func TestIsQuota429(t *testing.T) {
	resp := &http.Response{StatusCode: 429, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"quota exceeded","code":"insufficient_quota"}}`))}
	if !isQuota429(resp) {
		t.Fatal("expected quota 429")
	}
}

func TestIsQuota429NotGenericRateLimit(t *testing.T) {
	resp := &http.Response{StatusCode: 429, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"try again","code":"rate_limit_exceeded"}}`))}
	if isQuota429(resp) {
		t.Fatal("expected generic rate_limit_exceeded not to count as quota")
	}
}

func TestIsQuota429RestoresBody(t *testing.T) {
	const body = `{"error":{"message":"quota exceeded","code":"insufficient_quota"}}`
	resp := &http.Response{StatusCode: 429, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
	_ = isQuota429(resp)
	got, _ := io.ReadAll(resp.Body)
	if string(got) != body {
		t.Fatalf("body not restored: %q", string(got))
	}
}

func TestRequestTooLargeReturns413(t *testing.T) {
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"u"}})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(make([]byte, 20<<20+1)))
	req.Header.Set("Authorization", "Bearer p")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestDoUpstreamSetsDefaultUserAgent(t *testing.T) {
	var gotUA string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"u"}, UpstreamBaseURL: upstream.URL})
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	resp, err := app.doUpstream(req.Context(), req, nil, "u")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if gotUA != "OpenAI/Python 1.0.0" {
		t.Fatalf("got user-agent %q", gotUA)
	}
}
