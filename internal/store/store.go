// Package store persists the console's configuration: settings and SSH
// profiles, with secrets encrypted by the user's DPAPI on Windows.
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Profile is one saved SSH connection.
type Profile struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"` // 0 means 22
	User     string `json:"user"`
	AuthKind string `json:"auth_kind"` // "password" | "key"
	KeyPath  string `json:"key_path,omitempty"`
	// Secret holds the password, or the key's passphrase, encrypted by
	// DPAPI and encoded in base64; empty when the auth kind needs none.
	Secret string `json:"secret_dpapi,omitempty"`
}

// Settings are the console's own options.
type Settings struct {
	DefaultShell string `json:"default_shell,omitempty"`
	FontSize     int    `json:"font_size,omitempty"`
	MCPHTTP      bool   `json:"mcp_http"`
	MCPHTTPPort  int    `json:"mcp_http_port,omitempty"` // 0 means 8731
	// StrictHostKey is what an unknown SSH host key does:
	// "prompt" (ask, the GUI default), "accept_new" (store and go on,
	// the headless default), "off" (no verification, insecure).
	// A KNOWN host whose key CHANGED is always refused.
	StrictHostKey string `json:"strict_host_key,omitempty"`

	// FontFamily is the terminal font, a comma-separated list as the
	// terminal plugin takes it; empty means the system's monospace font.
	FontFamily string `json:"font_family,omitempty"`
	// TerminalTheme is a Ghostty color scheme by name; empty follows the
	// window's light or dark appearance.
	TerminalTheme string `json:"terminal_theme,omitempty"`
	// Appearance is the window's color scheme: "" follows the system,
	// "light" and "dark" force it. Terminals follow it too.
	Appearance string `json:"appearance,omitempty"`
	// RestoreDisabled turns the reopen-last-sessions behavior off.
	RestoreDisabled bool `json:"restore_disabled,omitempty"`

	// GuardDisabled turns the MCP command guardrail off (the default is
	// on — that is the point of it).
	GuardDisabled bool `json:"guard_disabled,omitempty"`
	// GuardHeadless decides Confirm-level calls with no human to ask
	// (stdio mode): "deny" (default) or "allow".
	GuardHeadless string `json:"guard_headless,omitempty"`
	// GuardDeny / GuardConfirm are custom regex patterns, on top of the
	// built-in rule base.
	GuardDeny    []string `json:"guard_deny,omitempty"`
	GuardConfirm []string `json:"guard_confirm,omitempty"`
}

// AuditLogPath is where the MCP audit trail lives.
func AuditLogPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "audit.log"), nil
}

// KnownHostsPath is where trusted SSH host keys live, OpenSSH format.
func KnownHostsPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "known_hosts"), nil
}

// RestoreSpec describes a session to reopen after a restart.
type RestoreSpec struct {
	Kind    string `json:"kind"` // "local" | "ssh"
	Shell   string `json:"shell,omitempty"`
	Cwd     string `json:"cwd,omitempty"`
	Custom  string `json:"custom,omitempty"`
	Profile string `json:"profile,omitempty"`
}

// Config is the whole configuration file.
type Config struct {
	Settings Settings      `json:"settings"`
	Profiles []Profile     `json:"ssh_profiles"`
	Restore  []RestoreSpec `json:"restore,omitempty"`
}

// Dir returns the configuration directory, %AppData%\adakaki.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "adakaki"), nil
}

// Path returns the configuration file's path.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load reads the configuration, an empty one when there is no file yet.
func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	cfg := &Config{Settings: Settings{MCPHTTP: true}}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("store: %s: %w", p, err)
	}
	return cfg, nil
}

// Save writes the configuration, creating the directory.
func (c *Config) Save() error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	p := filepath.Join(dir, "config.json")
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// Profile returns the profile with the name.
func (c *Config) Profile(name string) (Profile, bool) {
	for _, p := range c.Profiles {
		if p.Name == name {
			return p, true
		}
	}
	return Profile{}, false
}

// PutProfile adds or replaces a profile by name and saves.
func (c *Config) PutProfile(p Profile) error {
	for i := range c.Profiles {
		if c.Profiles[i].Name == p.Name {
			c.Profiles[i] = p
			return c.Save()
		}
	}
	c.Profiles = append(c.Profiles, p)
	return c.Save()
}

// DeleteProfile removes a profile by name and saves.
func (c *Config) DeleteProfile(name string) error {
	out := c.Profiles[:0]
	for _, p := range c.Profiles {
		if p.Name != name {
			out = append(out, p)
		}
	}
	c.Profiles = out
	return c.Save()
}
