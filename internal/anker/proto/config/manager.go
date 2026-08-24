package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ConfigManager manages loading and saving ankerctl configuration files.
type ConfigManager struct {
	configDir string
}

// NewConfigManager creates a new ConfigManager using the platform's
// user config directory (e.g. ~/.config/ankerctl on Linux).
func NewConfigManager() (*ConfigManager, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("get user config dir: %w", err)
	}
	return &ConfigManager{
		configDir: filepath.Join(configDir, "ankerctl"),
	}, nil
}

// NewConfigManagerWithDir creates a ConfigManager with a specific config directory.
func NewConfigManagerWithDir(dir string) *ConfigManager {
	return &ConfigManager{configDir: dir}
}

// ConfigPath returns the full path to a named config file.
func (m *ConfigManager) ConfigPath(name string) string {
	return filepath.Join(m.configDir, fmt.Sprintf("%s.json", name))
}

// ConfigDir returns the config directory.
func (m *ConfigManager) ConfigDir() string {
	return m.configDir
}

// Load reads a named config file. Returns nil if the file does not exist.
func (m *ConfigManager) Load(name string) (*Config, error) {
	path := m.ConfigPath(name)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{Printers: []Printer{}}, nil
		}
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	if cfg.Printers == nil {
		cfg.Printers = []Printer{}
	}

	return &cfg, nil
}

// Save writes a config to a named file.
func (m *ConfigManager) Save(name string, cfg *Config) error {
	if err := os.MkdirAll(m.configDir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	data = append(data, '\n')
	path := m.ConfigPath(name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	return nil
}

// Delete removes a named config file. Returns nil if the file does not exist.
func (m *ConfigManager) Delete(name string) error {
	path := m.ConfigPath(name)
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete config file: %w", err)
	}
	return nil
}

// LoadFromAPI fetches the configuration from the AnkerMake HTTP API
// using the provided auth token and region.
func LoadFromAPI(authToken, region string, insecure bool) (*Config, error) {
	return loadConfigFromAPI(authToken, region, insecure)
}

// LoadFromAPIWithUserID is like LoadFromAPI but also sets the Gtoken header
// using the provided user_id (MD5 hash). This is needed when the API requires
// the Gtoken header for authentication.
func LoadFromAPIWithUserID(authToken, region, userID string, insecure bool) (*Config, error) {
	return loadConfigFromAPIWithUserID(authToken, region, userID, insecure)
}
