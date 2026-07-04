package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func testHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	return dir
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.LLM.Active != "deepseek" {
		t.Fatalf("active = %q, want deepseek", cfg.LLM.Active)
	}
	if cfg.Policy.Default != "write_approval" {
		t.Fatalf("policy.default = %q, want write_approval", cfg.Policy.Default)
	}
	if len(cfg.Skills.Dirs) != 2 {
		t.Fatalf("skills.dirs len = %d, want 2", len(cfg.Skills.Dirs))
	}
}

func TestLoadSaveCreatesFileWithMode0600(t *testing.T) {
	home := testHome(t)
	store := NewConfigStoreAt(filepath.Join(home, ".databuff-diag", "config.yaml"))

	cfg, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LLM.Active != "deepseek" {
		t.Fatalf("active = %q, want deepseek", cfg.LLM.Active)
	}

	info, err := os.Stat(store.Path())
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != configFileMode {
		t.Fatalf("mode = %o, want %o", info.Mode().Perm(), configFileMode)
	}
}

func TestSavePreservesMode0600(t *testing.T) {
	home := testHome(t)
	path := filepath.Join(home, ".databuff-diag", "config.yaml")
	store := NewConfigStoreAt(path)

	if _, err := store.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	cfg, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.LLM.Active = "openai"
	cfg.LLM.Providers["openai"] = ProviderInstance{
		Enabled: true,
		APIKey:  "sk-secret",
		Model:   "gpt-4o",
	}
	if err := store.Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != configFileMode {
		t.Fatalf("mode = %o, want %o", info.Mode().Perm(), configFileMode)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	raw := string(data)
	if strings.Contains(raw, "sk-secret") {
		t.Fatalf("api_key must not be stored in plaintext on disk: %s", raw)
	}
	if !strings.Contains(raw, secretsEncryptedMarker()) {
		t.Fatal("expected encrypted api_key on disk")
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.LLM.Providers["openai"].APIKey != "sk-secret" {
		t.Fatalf("api_key = %q, want sk-secret", loaded.LLM.Providers["openai"].APIKey)
	}
}

func secretsEncryptedMarker() string {
	return "enc:v1:"
}

func TestSecretsEncryptedOnDisk(t *testing.T) {
	home := testHome(t)
	store := NewConfigStoreAt(filepath.Join(home, ".databuff-diag", "config.yaml"))

	cfg := DefaultConfig()
	cfg.LLM.Providers["deepseek"] = ProviderInstance{
		Enabled: true,
		APIKey:  "sk-encrypted-test",
		Model:   "deepseek-chat",
	}
	cfg.SSH.Hosts = SSHHostsList{
		{ID: NewSSHHostID(), Host: "10.0.0.8", User: "root", Password: "host-secret"},
	}
	if err := store.Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	raw, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	body := string(raw)
	if strings.Contains(body, "sk-encrypted-test") || strings.Contains(body, "host-secret") {
		t.Fatalf("secrets leaked in plaintext on disk: %s", body)
	}
	if !strings.Contains(body, secretsEncryptedMarker()) {
		t.Fatalf("expected encrypted secrets on disk: %s", body)
	}
}

func TestLoadMigratesLegacyPlaintextSecrets(t *testing.T) {
	home := testHome(t)
	path := filepath.Join(home, ".databuff-diag", "config.yaml")
	store := NewConfigStoreAt(path)

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	raw := []byte(`llm:
  active: deepseek
  providers:
    deepseek:
      enabled: true
      api_key: legacy-plain-key
      model: deepseek-chat
policy:
  default: write_approval
ssh:
  hosts:
    - id: host-legacy
      host: 10.0.0.9
      user: root
      password: legacy-host-pass
skills:
  dirs: []
auth:
  username: Admin
  password: Databuff@123
`)
	if err := os.WriteFile(path, raw, configFileMode); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.LLM.Providers["deepseek"].APIKey != "legacy-plain-key" {
		t.Fatalf("api_key = %q, want legacy-plain-key", loaded.LLM.Providers["deepseek"].APIKey)
	}
	if len(loaded.SSH.Hosts) != 1 || loaded.SSH.Hosts[0].Password != "legacy-host-pass" {
		t.Fatalf("host password = %+v", loaded.SSH.Hosts)
	}

	disk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	body := string(disk)
	if strings.Contains(body, "legacy-plain-key") || strings.Contains(body, "legacy-host-pass") {
		t.Fatalf("legacy secrets still plaintext on disk after migration: %s", body)
	}
}

func TestRoundTripYAML(t *testing.T) {
	home := testHome(t)
	store := NewConfigStoreAt(filepath.Join(home, ".databuff-diag", "config.yaml"))

	cfg := DefaultConfig()
	cfg.LLM.Providers["anthropic"] = ProviderInstance{
		Enabled:    true,
		WireAPI:    "anthropic",
		APIKey:     "anthropic-key",
		Model:      "claude-sonnet-4-20250514",
		TimeoutSec: 90,
	}
	if err := store.Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.LLM.Providers["anthropic"].APIKey != "anthropic-key" {
		t.Fatalf("api_key = %q, want anthropic-key", loaded.LLM.Providers["anthropic"].APIKey)
	}

	raw, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
}

func TestLoadMigratesLegacyStringHosts(t *testing.T) {
	home := testHome(t)
	path := filepath.Join(home, ".databuff-diag", "config.yaml")
	store := NewConfigStoreAt(path)

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	raw := []byte(`ssh:
  control_path: /tmp/ssh-%r@%h-%p
  control_persist: 10m
  hosts:
    - 192.168.1.10
    - db.example.com
`)
	if err := os.WriteFile(path, raw, configFileMode); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.SSH.Hosts) != 2 {
		t.Fatalf("hosts len = %d, want 2", len(cfg.SSH.Hosts))
	}
	if cfg.SSH.Hosts[0].Host != "192.168.1.10" || cfg.SSH.Hosts[0].ID == "" {
		t.Fatalf("host[0] = %+v, want migrated host with id", cfg.SSH.Hosts[0])
	}
	if cfg.SSH.Hosts[1].Host != "db.example.com" {
		t.Fatalf("host[1].Host = %q, want db.example.com", cfg.SSH.Hosts[1].Host)
	}
}
