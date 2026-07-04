package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/databufflabs/databuff-diag/internal/config"
	"github.com/databufflabs/databuff-diag/internal/secrets"
	"gopkg.in/yaml.v3"
)

const configFileName = "config.yaml"
const configFileMode = 0o600

// Config is the user configuration persisted at ~/.databuff-diag/config.yaml.
type Config struct {
	LLM      LLMConfig      `yaml:"llm" json:"llm"`
	Policy   PolicyConfig   `yaml:"policy" json:"policy"`
	SSH      SSHConfig      `yaml:"ssh" json:"ssh"`
	Skills   SkillsConfig   `yaml:"skills" json:"skills"`
	Auth     AuthConfig     `yaml:"auth" json:"auth"`
	Sessions SessionsConfig `yaml:"sessions" json:"sessions"`
}

// SessionsConfig controls session retention and scheduled cleanup.
type SessionsConfig struct {
	RetentionDays *int `yaml:"retention_days,omitempty" json:"retention_days,omitempty"`
	CleanupHour   *int `yaml:"cleanup_hour,omitempty" json:"cleanup_hour,omitempty"`
}

// RetentionDaysOrDefault returns retention days; unset defaults to 30; 0 disables cleanup.
func (c SessionsConfig) RetentionDaysOrDefault() int {
	if c.RetentionDays == nil {
		return 30
	}
	return *c.RetentionDays
}

// CleanupEnabled reports whether automatic session cleanup should run.
func (c SessionsConfig) CleanupEnabled() bool {
	return c.RetentionDaysOrDefault() > 0
}

// CleanupHourLocal returns the local hour (0–23) for the daily cleanup job.
func (c SessionsConfig) CleanupHourLocal() int {
	if c.CleanupHour != nil {
		h := *c.CleanupHour
		if h >= 0 && h <= 23 {
			return h
		}
	}
	return 1
}

// AuthConfig holds login credentials for the web UI.
type AuthConfig struct {
	Username string `yaml:"username" json:"username"`
	Password string `yaml:"password" json:"-"`
}

// LLMConfig holds the active provider and per-provider overrides.
type LLMConfig struct {
	Active    string                      `yaml:"active" json:"active"`
	Providers map[string]ProviderInstance `yaml:"providers" json:"providers"`
}

// ProviderInstance is a user-configured LLM provider.
type ProviderInstance struct {
	Enabled           bool   `yaml:"enabled" json:"enabled"`
	WireAPI           string `yaml:"wire_api,omitempty" json:"wire_api,omitempty"`
	BaseURL           string `yaml:"base_url,omitempty" json:"base_url,omitempty"`
	APIKey            string `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	Model             string `yaml:"model,omitempty" json:"model,omitempty"`
	TimeoutSec        int    `yaml:"timeout_sec,omitempty" json:"timeout_sec,omitempty"`
	ResponseProcessor string `yaml:"response_processor,omitempty" json:"response_processor,omitempty"`
	SupportsVision    *bool  `yaml:"supports_vision,omitempty" json:"supports_vision,omitempty"`
}

// PolicyConfig holds command approval defaults.
type PolicyConfig struct {
	Default string `yaml:"default" json:"default"`
}

// SSHHost is a saved remote host with login credentials.
type SSHHost struct {
	ID                 string `yaml:"id" json:"id"`
	Name               string `yaml:"name,omitempty" json:"name,omitempty"`
	Host               string `yaml:"host" json:"host"`
	Port               int    `yaml:"port,omitempty" json:"port,omitempty"`
	User               string `yaml:"user" json:"user"`
	Password           string `yaml:"password,omitempty" json:"password,omitempty"`
	PasswordConfigured bool   `yaml:"-" json:"password_configured,omitempty"`
}

// SSHHostsList is persisted as ssh.hosts; accepts legacy []string entries on load.
type SSHHostsList []SSHHost

// UnmarshalYAML supports both legacy string hosts and structured SSHHost entries.
func (list *SSHHostsList) UnmarshalYAML(node *yaml.Node) error {
	if node == nil || node.Kind == yaml.DocumentNode {
		if len(node.Content) > 0 {
			node = node.Content[0]
		}
	}
	if node == nil || node.ShortTag() == "!!null" {
		*list = SSHHostsList{}
		return nil
	}
	if node.Kind != yaml.SequenceNode {
		return fmt.Errorf("ssh.hosts: expected sequence")
	}
	if len(node.Content) == 0 {
		*list = SSHHostsList{}
		return nil
	}
	if node.Content[0].Kind == yaml.ScalarNode {
		var hosts []string
		if err := node.Decode(&hosts); err != nil {
			return err
		}
		out := make(SSHHostsList, 0, len(hosts))
		for _, h := range hosts {
			out = append(out, SSHHost{ID: NewSSHHostID(), Host: h})
		}
		*list = out
		return nil
	}
	var hosts []SSHHost
	if err := node.Decode(&hosts); err != nil {
		return err
	}
	for i := range hosts {
		if hosts[i].ID == "" {
			hosts[i].ID = NewSSHHostID()
		}
	}
	*list = SSHHostsList(hosts)
	return nil
}

// NewSSHHostID returns a stable random id for CRUD operations.
func NewSSHHostID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("host-%d", os.Getpid())
	}
	return "host-" + hex.EncodeToString(b)
}

// SSHConfig holds SSH connection defaults.
type SSHConfig struct {
	ControlPath    string       `yaml:"control_path" json:"control_path"`
	ControlPersist string       `yaml:"control_persist" json:"control_persist"`
	Hosts          SSHHostsList `yaml:"hosts" json:"hosts"`
}

// SkillsConfig lists skill directories to load.
type SkillsConfig struct {
	Dirs []string `yaml:"dirs" json:"dirs"`
}

// ConfigStore loads and saves ~/.databuff-diag/config.yaml.
type ConfigStore struct {
	path       string
	vaultCache *secrets.Vault
}

// NewConfigStore resolves the default config file path under the user home dir.
func NewConfigStore() (*ConfigStore, error) {
	home, err := config.HomeDir()
	if err != nil {
		return nil, err
	}
	return &ConfigStore{path: filepath.Join(home, configFileName)}, nil
}

// NewConfigStoreAt creates a store for tests with an explicit file path.
func NewConfigStoreAt(path string) *ConfigStore {
	return &ConfigStore{path: path}
}

// Path returns the config file path.
func (s *ConfigStore) Path() string {
	return s.path
}

// DefaultConfig returns first-run defaults aligned with design.md §5.3.
func DefaultConfig() *Config {
	home, err := config.HomeDir()
	if err != nil {
		home = filepath.Join("~", config.DirName)
	}
	cleanupHour := 1
	retentionDays := 30
	return &Config{
		LLM: LLMConfig{
			Active: "",
			Providers: map[string]ProviderInstance{
				"deepseek": {
					Enabled:    false,
					WireAPI:    "openai_compat",
					BaseURL:    "https://api.deepseek.com/v1",
					Model:      "deepseek-v4-flash",
					TimeoutSec: 120,
				},
			},
		},
		Policy: PolicyConfig{Default: "open"},
		SSH: SSHConfig{
			ControlPath:    filepath.Join(home, "ssh", "%r@%h-%p"),
			ControlPersist: "10m",
			Hosts:          SSHHostsList{},
		},
		Skills: SkillsConfig{
			Dirs: []string{
				filepath.Join(home, "skills"),
				"./deploy/skills",
			},
		},
		Auth: AuthConfig{
			Username: "Admin",
			Password: "Databuff@123",
		},
		Sessions: SessionsConfig{
			RetentionDays: &retentionDays,
			CleanupHour:   &cleanupHour,
		},
	}
}

// normalizeLLMActive clears active when the provider is missing or not enabled.
func normalizeLLMActive(cfg *Config) bool {
	if cfg == nil || cfg.LLM.Active == "" {
		return false
	}
	inst, ok := cfg.LLM.Providers[cfg.LLM.Active]
	if ok && inst.Enabled {
		return false
	}
	cfg.LLM.Active = ""
	return true
}

// Load reads config from disk, returning defaults when the file is missing.
func (s *ConfigStore) Load() (*Config, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := DefaultConfig()
			if saveErr := s.Save(cfg); saveErr != nil {
				return nil, saveErr
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.LLM.Providers == nil {
		cfg.LLM.Providers = map[string]ProviderInstance{}
	}
	if cfg.SSH.Hosts == nil {
		cfg.SSH.Hosts = SSHHostsList{}
	}
	for i := range cfg.SSH.Hosts {
		if cfg.SSH.Hosts[i].ID == "" {
			cfg.SSH.Hosts[i].ID = NewSSHHostID()
		}
	}
	if cfg.Skills.Dirs == nil {
		cfg.Skills.Dirs = []string{}
	}
	if cfg.Auth.Username == "" {
		cfg.Auth.Username = "Admin"
	}
	if cfg.Auth.Password == "" {
		cfg.Auth.Password = "Databuff@123"
	}
	needsNormalize := normalizeLLMActive(cfg)
	needsMigrate := s.hasPlaintextSecrets(cfg)
	if err := s.decryptConfigSecrets(cfg); err != nil {
		return nil, err
	}
	if needsMigrate || needsNormalize {
		if saveErr := s.Save(cfg); saveErr != nil {
			return nil, saveErr
		}
	}
	return cfg, nil
}

func (s *ConfigStore) vault() (*secrets.Vault, error) {
	if s.vaultCache != nil {
		return s.vaultCache, nil
	}
	v, err := secrets.NewVaultAt(filepath.Dir(s.path))
	if err != nil {
		return nil, err
	}
	s.vaultCache = v
	return v, nil
}

func (s *ConfigStore) decryptConfigSecrets(cfg *Config) error {
	if cfg == nil {
		return nil
	}
	vault, err := s.vault()
	if err != nil {
		return err
	}
	for code, inst := range cfg.LLM.Providers {
		if inst.APIKey == "" {
			continue
		}
		plain, err := vault.Decrypt(inst.APIKey)
		if err != nil {
			return fmt.Errorf("decrypt llm.providers.%s.api_key: %w", code, err)
		}
		inst.APIKey = plain
		cfg.LLM.Providers[code] = inst
	}
	for i := range cfg.SSH.Hosts {
		if cfg.SSH.Hosts[i].Password == "" {
			continue
		}
		plain, err := vault.Decrypt(cfg.SSH.Hosts[i].Password)
		if err != nil {
			return fmt.Errorf("decrypt ssh.hosts[%d].password: %w", i, err)
		}
		cfg.SSH.Hosts[i].Password = plain
	}
	return nil
}

func (s *ConfigStore) encryptConfigSecrets(cfg *Config) error {
	if cfg == nil {
		return nil
	}
	vault, err := s.vault()
	if err != nil {
		return err
	}
	for code, inst := range cfg.LLM.Providers {
		if inst.APIKey == "" {
			continue
		}
		enc, err := vault.Encrypt(inst.APIKey)
		if err != nil {
			return fmt.Errorf("encrypt llm.providers.%s.api_key: %w", code, err)
		}
		inst.APIKey = enc
		cfg.LLM.Providers[code] = inst
	}
	for i := range cfg.SSH.Hosts {
		if cfg.SSH.Hosts[i].Password == "" {
			continue
		}
		enc, err := vault.Encrypt(cfg.SSH.Hosts[i].Password)
		if err != nil {
			return fmt.Errorf("encrypt ssh.hosts[%d].password: %w", i, err)
		}
		cfg.SSH.Hosts[i].Password = enc
	}
	return nil
}

func (s *ConfigStore) hasPlaintextSecrets(cfg *Config) bool {
	if cfg == nil {
		return false
	}
	for _, inst := range cfg.LLM.Providers {
		if inst.APIKey != "" && !secrets.IsEncrypted(inst.APIKey) {
			return true
		}
	}
	for _, h := range cfg.SSH.Hosts {
		if h.Password != "" && !secrets.IsEncrypted(h.Password) {
			return true
		}
	}
	return false
}

// Save writes config with file mode 0600.
func (s *ConfigStore) Save(cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	onDisk := cloneConfig(cfg)
	if err := s.encryptConfigSecrets(onDisk); err != nil {
		return err
	}

	data, err := yaml.Marshal(onDisk)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(s.path, data, configFileMode); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Chmod(s.path, configFileMode); err != nil {
		return fmt.Errorf("chmod config: %w", err)
	}
	return nil
}

func cloneConfig(cfg *Config) *Config {
	if cfg == nil {
		return nil
	}
	out := *cfg
	if cfg.LLM.Providers != nil {
		out.LLM.Providers = make(map[string]ProviderInstance, len(cfg.LLM.Providers))
		for code, inst := range cfg.LLM.Providers {
			out.LLM.Providers[code] = inst
		}
	}
	if cfg.SSH.Hosts != nil {
		out.SSH.Hosts = make(SSHHostsList, len(cfg.SSH.Hosts))
		copy(out.SSH.Hosts, cfg.SSH.Hosts)
	}
	if cfg.Skills.Dirs != nil {
		out.Skills.Dirs = append([]string(nil), cfg.Skills.Dirs...)
	}
	return &out
}
