package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type KeyEntry struct {
	Key  string `json:"key"`
	Name string `json:"name,omitempty"`
}

func (ke *KeyEntry) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		ke.Key = s
		return nil
	}
	var raw struct {
		Key  string `json:"key"`
		Name string `json:"name,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	ke.Key = raw.Key
	ke.Name = raw.Name
	return nil
}

func KeyEntriesToKeys(entries []KeyEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Key
	}
	return out
}

func KeyEntriesToNames(entries []KeyEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Name
	}
	return out
}

type Config struct {
	ListenAddr          string     `json:"listen_addr"`
	UpstreamBaseURL     string     `json:"upstream_base_url"`
	ProxyAPIKey         string     `json:"proxy_api_key,omitempty"`
	UpstreamAPIKeys     []KeyEntry `json:"upstream_api_keys"`
	MaxRequestBodyBytes int64      `json:"max_request_body_bytes"`
	RequestLogSize      int        `json:"request_log_size"`
	DockerComposePath   string     `json:"-"`
	SMTPHost            string     `json:"smtp_host"`
	SMTPPort            int        `json:"smtp_port"`
	SMTPUsername        string     `json:"smtp_username"`
	SMTPPassword        string     `json:"smtp_password"`
	SMTPFrom            string     `json:"smtp_from"`
	SMTPTo              string     `json:"smtp_to"`
	SMTPTLS             bool       `json:"smtp_tls"`
	SMTPStartTLS        bool       `json:"smtp_starttls"`
}

func DefaultConfig() Config {
	return Config{
		ListenAddr:          "0.0.0.0:8080",
		UpstreamBaseURL:     "https://opencode.ai/zen/go/v1",
		MaxRequestBodyBytes: 20 << 20,
		RequestLogSize:      500,
		SMTPPort:            25,
		DockerComposePath:   "/docker-compose.yml",
	}
}

func ConfigPath() string {
	if p := strings.TrimSpace(os.Getenv("SWITCHBOARD_GO_CONFIG")); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		return filepath.Join(home, ".config", "switchboard-go", "config.json")
	}
	return ""
}

func LoadConfig() (Config, error) {
	cfg := DefaultConfig()
	path := ConfigPath()

	// Load existing config file (may include proxy_api_key now)
	if path != "" {
		data, err := os.ReadFile(path)
		if err == nil {
			json.Unmarshal(data, &cfg)
		} else if !os.IsNotExist(err) {
			return Config{}, fmt.Errorf("read config: %w", err)
		}
	}

	// Env overrides config file
	if proxyKey := strings.TrimSpace(os.Getenv("PROXY_API_KEY")); proxyKey != "" {
		cfg.ProxyAPIKey = proxyKey
	}
	if cp := strings.TrimSpace(os.Getenv("COMPOSE_FILE_PATH")); cp != "" {
		cfg.DockerComposePath = cp
	}

	if !cfg.ProxyAPIKeySet() {
		return Config{}, fmt.Errorf("PROXY_API_KEY is required (set PROXY_API_KEY env var or proxy_api_key in config file)")
	}

	// First-run: seed from env and save
	if path != "" {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if seed := strings.TrimSpace(os.Getenv("OPENCODE_GO_API_KEYS")); seed != "" {
				for _, k := range strings.Split(seed, ",") {
					if s := strings.TrimSpace(k); s != "" {
						cfg.UpstreamAPIKeys = append(cfg.UpstreamAPIKeys, KeyEntry{Key: s})
					}
				}
			}
			if saveErr := SaveConfig(cfg, path); saveErr != nil {
				return cfg, saveErr
			}
		}
	}

	return cfg, nil
}

func LoadConfigFromBytes(data []byte) (Config, error) {
	cfg := DefaultConfig()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

func SaveConfig(cfg Config, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("mkdir config dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write config tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename config: %w", err)
	}
	return nil
}

func (c Config) Validate() error {
	if !c.ProxyAPIKeySet() {
		return fmt.Errorf("PROXY_API_KEY is required")
	}
	if c.UpstreamBaseURL == "" {
		return fmt.Errorf("upstream_base_url is required")
	}
	if c.MaxRequestBodyBytes <= 0 {
		return fmt.Errorf("max_request_body_bytes must be > 0")
	}
	return nil
}

func (c Config) ProxyAPIKeySet() bool {
	return strings.TrimSpace(c.ProxyAPIKey) != ""
}

func (c Config) SMTPConfigured() bool {
	return c.SMTPHost != "" && c.SMTPFrom != "" && c.SMTPTo != ""
}
