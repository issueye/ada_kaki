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

	// AI configures the built-in AI operator (see
	// docs/AI-功能规划与设计.md §7.1). A missing section means the
	// documented defaults, so configs written before this feature keep
	// working and Normalize fills the gaps.
	AI AISettings `json:"ai,omitempty"`
}

// AI provider names.
const (
	AIProviderLocal = "local"
	AIProviderCloud = "cloud"
)

// Defaults of the AI operator; docs/AI-功能规划与设计.md §7.1 is the source.
const (
	DefaultAIBaseURL       = "http://127.0.0.1:3050/v1"
	DefaultAIModel         = "deepseek-v4.1-flash-eco"
	DefaultAIKeyEnv        = "OPENDESIGN_API_KEY"
	DefaultAIContextWindow = 1_000_000
	DefaultAIMaxToolCalls  = 40
	DefaultAISessionTokens = 200_000
	DefaultAIDockWidth     = 340
	AIDockWidthMin         = 280
	AIDockWidthMax         = 420
	DefaultAIRetentionDays = 30
)

// AISettings is the AI operator's configuration. The *bool fields default
// to true and stay pointers so an explicit false survives a round trip.
type AISettings struct {
	Enabled   bool   `json:"enabled,omitempty"`
	Provider  string `json:"provider,omitempty"` // "local" | "cloud"
	BaseURL   string `json:"base_url,omitempty"`
	Model     string `json:"model,omitempty"`
	Reasoning string `json:"reasoning,omitempty"` // "off" | "low" | "medium" | "high"
	APIKeyEnv string `json:"api_key_env,omitempty"`

	ContextWindow     int `json:"context_window,omitempty"`
	MaxToolCalls      int `json:"max_tool_calls,omitempty"`
	SessionTokenLimit int `json:"session_token_limit,omitempty"`

	DockOpen    *bool `json:"dock_open,omitempty"`
	DockWidth   int   `json:"dock_width,omitempty"`
	LockInput   *bool `json:"lock_input,omitempty"`
	AutoRestore *bool `json:"auto_restore,omitempty"`

	ToolScope string `json:"tool_scope,omitempty"` // "all" | "write" | "readonly"
	GuardMode string `json:"guard_mode,omitempty"` // "confirm_all" | "high_only" | "allow"

	AuditEnabled       *bool `json:"audit_enabled,omitempty"`
	AuditRedact        *bool `json:"audit_redact,omitempty"`
	AuditRetentionDays int   `json:"audit_retention_days,omitempty"`
}

func boolPtr(v bool) *bool { return &v }

// Normalize fills absent values with the documented defaults and clamps
// numeric fields into range. It is idempotent and runs on every Load.
func (a *AISettings) Normalize() {
	if a.Provider != AIProviderCloud {
		a.Provider = AIProviderLocal
	}
	if a.Provider == AIProviderLocal && a.BaseURL == "" {
		a.BaseURL = DefaultAIBaseURL
	}
	if a.Model == "" {
		a.Model = DefaultAIModel
	}
	switch a.Reasoning {
	case "off", "low", "medium", "high":
	default:
		a.Reasoning = "high"
	}
	if a.APIKeyEnv == "" {
		a.APIKeyEnv = DefaultAIKeyEnv
	}

	if a.ContextWindow <= 0 {
		a.ContextWindow = DefaultAIContextWindow
	}
	if a.MaxToolCalls <= 0 {
		a.MaxToolCalls = DefaultAIMaxToolCalls
	}
	if a.MaxToolCalls > 200 {
		a.MaxToolCalls = 200
	}
	if a.SessionTokenLimit <= 0 {
		a.SessionTokenLimit = DefaultAISessionTokens
	}
	if a.SessionTokenLimit < 1_000 {
		a.SessionTokenLimit = 1_000
	}
	if a.SessionTokenLimit > 1_000_000 {
		a.SessionTokenLimit = 1_000_000
	}

	if a.DockOpen == nil {
		a.DockOpen = boolPtr(true)
	}
	if a.DockWidth <= 0 {
		a.DockWidth = DefaultAIDockWidth
	}
	if a.DockWidth < AIDockWidthMin {
		a.DockWidth = AIDockWidthMin
	}
	if a.DockWidth > AIDockWidthMax {
		a.DockWidth = AIDockWidthMax
	}
	if a.LockInput == nil {
		a.LockInput = boolPtr(true)
	}
	if a.AutoRestore == nil {
		a.AutoRestore = boolPtr(true)
	}

	switch a.ToolScope {
	case "all", "write", "readonly":
	default:
		a.ToolScope = "all"
	}
	switch a.GuardMode {
	case "confirm_all", "high_only", "allow":
	default:
		a.GuardMode = "confirm_all"
	}

	if a.AuditEnabled == nil {
		a.AuditEnabled = boolPtr(true)
	}
	if a.AuditRedact == nil {
		a.AuditRedact = boolPtr(true)
	}
	if a.AuditRetentionDays <= 0 {
		a.AuditRetentionDays = DefaultAIRetentionDays
	}
	if a.AuditRetentionDays > 3_650 {
		a.AuditRetentionDays = 3_650
	}
}

// Validate reports values Normalize cannot repair: enums after a hand
// edit, and a cloud provider missing its endpoint.
func (a AISettings) Validate() error {
	if a.Provider != AIProviderLocal && a.Provider != AIProviderCloud {
		return fmt.Errorf("ai: provider %q must be %q or %q", a.Provider, AIProviderLocal, AIProviderCloud)
	}
	if a.Provider == AIProviderCloud && a.BaseURL == "" {
		return fmt.Errorf("ai: base_url is required for the cloud provider")
	}
	switch a.Reasoning {
	case "off", "low", "medium", "high":
	default:
		return fmt.Errorf("ai: reasoning %q must be off|low|medium|high", a.Reasoning)
	}
	switch a.ToolScope {
	case "all", "write", "readonly":
	default:
		return fmt.Errorf("ai: tool_scope %q must be all|write|readonly", a.ToolScope)
	}
	switch a.GuardMode {
	case "confirm_all", "high_only", "allow":
	default:
		return fmt.Errorf("ai: guard_mode %q must be confirm_all|high_only|allow", a.GuardMode)
	}
	if a.MaxToolCalls < 1 || a.MaxToolCalls > 200 {
		return fmt.Errorf("ai: max_tool_calls %d must be 1–200", a.MaxToolCalls)
	}
	if a.SessionTokenLimit < 1_000 || a.SessionTokenLimit > 1_000_000 {
		return fmt.Errorf("ai: session_token_limit %d must be 1k–1M", a.SessionTokenLimit)
	}
	if a.DockWidth < AIDockWidthMin || a.DockWidth > AIDockWidthMax {
		return fmt.Errorf("ai: dock_width %d must be %d–%d", a.DockWidth, AIDockWidthMin, AIDockWidthMax)
	}
	return nil
}

// The boolean accessors keep callers free of nil checks after Normalize.

func (a AISettings) DockOpenOn() bool     { return a.DockOpen == nil || *a.DockOpen }
func (a AISettings) LockInputOn() bool    { return a.LockInput == nil || *a.LockInput }
func (a AISettings) AutoRestoreOn() bool  { return a.AutoRestore == nil || *a.AutoRestore }
func (a AISettings) AuditOn() bool        { return a.AuditEnabled == nil || *a.AuditEnabled }
func (a AISettings) AuditRedactOn() bool  { return a.AuditRedact == nil || *a.AuditRedact }

// Normalize fills the whole settings document's defaults.
func (s *Settings) Normalize() {
	s.AI.Normalize()
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
		cfg.Settings.Normalize()
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("store: %s: %w", p, err)
	}
	cfg.Settings.Normalize()
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
