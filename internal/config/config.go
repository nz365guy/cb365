package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	AppName    = "cb365"
	AppVersion = "0.1.0-dev"
)

// AuthMode represents the authentication mode
type AuthMode string

const (
	AuthModeDelegated AuthMode = "delegated"
	AuthModeAppOnly   AuthMode = "app-only"
)

// Profile represents a configured authentication profile
type Profile struct {
	Name             string                    `json:"name"`
	TenantID         string                    `json:"tenant_id"`
	ClientID         string                    `json:"client_id"`
	AuthMode         AuthMode                  `json:"auth_mode"`
	Scopes           []string                  `json:"scopes,omitempty"`
	Username         string                    `json:"username,omitempty"`
	Active           bool                      `json:"active,omitempty"`
	ManagedDelegated *ManagedDelegatedMetadata `json:"managed_delegated,omitempty"`
}

// ManagedDelegatedMetadata contains non-secret references for a delegated
// profile whose bearer material lives exclusively in Bitwarden Secrets
// Manager EU. None of these fields is sufficient to authenticate to either
// Bitwarden or Entra.
type ManagedDelegatedMetadata struct {
	HomeAccountID            string            `json:"home_account_id"`
	SecretID                 string            `json:"secret_id"`
	OrganisationID           string            `json:"organisation_id"`
	ProjectID                string            `json:"project_id"`
	AssignedHost             string            `json:"assigned_host"`
	MigrationState           string            `json:"migration_state"`
	ChannelMessageProvenance map[string]string `json:"channel_message_provenance,omitempty"`
}

// Config represents the cb365 configuration file
type Config struct {
	ActiveProfile string              `json:"active_profile"`
	Profiles      map[string]*Profile `json:"profiles"`
	Settings      Settings            `json:"settings"`
}

// Settings holds global CLI settings
type Settings struct {
	IPv4Only bool `json:"ipv4_only,omitempty"`
}

// ConfigDir returns the config directory path
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", AppName), nil
}

// ConfigPath returns the full path to the config file
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load reads the config from disk
func Load() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path) // #nosec G304 — path from internal ConfigPath(), not user input
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{
				Profiles: make(map[string]*Profile),
			}, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = make(map[string]*Profile)
	}
	return &cfg, nil
}

// Save writes the config to disk
func (c *Config) Save() error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}

	tmpFile, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("creating config temporary file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath) // #nosec G104 -- best-effort cleanup
	if err := tmpFile.Chmod(0600); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("restricting config temporary file: %w", err)
	}
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("writing config: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("syncing config: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("closing config temporary file: %w", err)
	}
	if err := replaceFile(tmpPath, path); err != nil {
		return fmt.Errorf("replacing config: %w", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		return fmt.Errorf("restricting config: %w", err)
	}
	return nil
}

// ActiveProfileConfig returns the currently active profile
func (c *Config) ActiveProfileConfig() (*Profile, error) {
	if c.ActiveProfile == "" {
		return nil, fmt.Errorf("no active profile set — run 'cb365 auth login' first")
	}
	p, ok := c.Profiles[c.ActiveProfile]
	if !ok {
		return nil, fmt.Errorf("active profile %q not found in config", c.ActiveProfile)
	}
	return p, nil
}

// SetActiveProfile switches the active profile
func (c *Config) SetActiveProfile(name string) error {
	if _, ok := c.Profiles[name]; !ok {
		return fmt.Errorf("profile %q not found", name)
	}
	for _, p := range c.Profiles {
		p.Active = false
	}
	c.ActiveProfile = name
	c.Profiles[name].Active = true
	return nil
}
