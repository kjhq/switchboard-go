package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.ListenAddr != "0.0.0.0:8080" {
		t.Fatalf("expected 0.0.0.0:8080, got %s", cfg.ListenAddr)
	}
	if cfg.UpstreamBaseURL != "https://opencode.ai/zen/go/v1" {
		t.Fatalf("unexpected base url")
	}
	if cfg.MaxRequestBodyBytes != 20<<20 {
		t.Fatalf("unexpected max body bytes")
	}
	if cfg.RequestLogSize != 500 {
		t.Fatalf("unexpected request log size")
	}
}

func TestLoadConfigCreatesDefaultsOnFirstRun(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SWITCHBOARD_GO_CONFIG", filepath.Join(dir, "config.json"))
	t.Setenv("PROXY_API_KEY", "test-proxy-key")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != "0.0.0.0:8080" {
		t.Fatalf("expected defaults after first run")
	}
	if len(cfg.UpstreamAPIKeys) != 0 {
		t.Fatalf("expected 0 keys on first run")
	}
}

func TestLoadConfigSeedsFromEnvOnFirstRun(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SWITCHBOARD_GO_CONFIG", filepath.Join(dir, "config.json"))
	t.Setenv("PROXY_API_KEY", "test-proxy-key")
	t.Setenv("OPENCODE_GO_API_KEYS", "sk-seed1,sk-seed2")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.UpstreamAPIKeys) != 2 {
		t.Fatalf("expected 2 seeded keys, got %d", len(cfg.UpstreamAPIKeys))
	}
	if cfg.UpstreamAPIKeys[0].Key != "sk-seed1" || cfg.UpstreamAPIKeys[1].Key != "sk-seed2" {
		t.Fatalf("unexpected seed keys: %v", cfg.UpstreamAPIKeys)
	}
}

func TestLoadConfigIgnoresEnvOnSubsequentRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	initial := `{"listen_addr": "127.0.0.1:9090", "upstream_api_keys": ["sk-existing"]}`
	if err := os.WriteFile(path, []byte(initial), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SWITCHBOARD_GO_CONFIG", path)
	t.Setenv("PROXY_API_KEY", "test-proxy-key")
	t.Setenv("OPENCODE_GO_API_KEYS", "sk-should-be-ignored")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != "127.0.0.1:9090" {
		t.Fatalf("expected 127.0.0.1:9090, got %s", cfg.ListenAddr)
	}
	if len(cfg.UpstreamAPIKeys) != 1 || cfg.UpstreamAPIKeys[0].Key != "sk-existing" {
		t.Fatalf("expected seeded keys, got %v", cfg.UpstreamAPIKeys)
	}
}

func TestSaveConfigAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := DefaultConfig()
	cfg.ListenAddr = "0.0.0.0:9999"
	if err := SaveConfig(cfg, path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadConfigFromBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ListenAddr != "0.0.0.0:9999" {
		t.Fatalf("expected 0.0.0.0:9999, got %s", loaded.ListenAddr)
	}
}

func TestConfigValidateRequiresProxyKey(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error without PROXY_API_KEY")
	}
	cfg.ProxyAPIKey = "p"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config, got: %v", err)
	}
}

func TestConfigValidateRequiresUpstreamURL(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ProxyAPIKey = "p"
	cfg.UpstreamBaseURL = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error without upstream base URL")
	}
}

func TestConfigValidateRequiresMaxBodyBytes(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ProxyAPIKey = "p"
	cfg.MaxRequestBodyBytes = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error without max body bytes")
	}
}

func TestKeyEntryUnmarshalString(t *testing.T) {
	var ke KeyEntry
	if err := ke.UnmarshalJSON([]byte(`"sk-abc"`)); err != nil {
		t.Fatal(err)
	}
	if ke.Key != "sk-abc" || ke.Name != "" {
		t.Fatalf("unexpected: %+v", ke)
	}
}

func TestKeyEntryUnmarshalObject(t *testing.T) {
	var ke KeyEntry
	if err := ke.UnmarshalJSON([]byte(`{"key":"sk-abc","name":"my key"}`)); err != nil {
		t.Fatal(err)
	}
	if ke.Key != "sk-abc" || ke.Name != "my key" {
		t.Fatalf("unexpected: %+v", ke)
	}
}

func TestKeyEntryMarshal(t *testing.T) {
	ke := KeyEntry{Key: "sk-abc", Name: "my key"}
	data, err := json.Marshal(ke)
	if err != nil {
		t.Fatal(err)
	}
	var restored KeyEntry
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Key != "sk-abc" || restored.Name != "my key" {
		t.Fatalf("unexpected: %+v", restored)
	}
}
