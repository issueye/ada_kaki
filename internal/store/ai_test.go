package store

import (
	"encoding/json"
	"strings"
	"testing"
)

// The AI section defaults to the documented values, so a config written
// before this feature keeps working.
func TestAINormalizeDefaults(t *testing.T) {
	var a AISettings
	a.Normalize()

	if a.Provider != AIProviderLocal {
		t.Fatalf("provider = %q, want %q", a.Provider, AIProviderLocal)
	}
	if a.Provider == AIProviderLocal && a.BaseURL != DefaultAIBaseURL {
		t.Fatalf("base_url = %q, want %q", a.BaseURL, DefaultAIBaseURL)
	}
	if a.Model != DefaultAIModel {
		t.Fatalf("model = %q, want %q", a.Model, DefaultAIModel)
	}
	if a.Reasoning != "high" {
		t.Fatalf("reasoning = %q, want high", a.Reasoning)
	}
	if a.APIKeyEnv != DefaultAIKeyEnv {
		t.Fatalf("api_key_env = %q, want %q", a.APIKeyEnv, DefaultAIKeyEnv)
	}
	if a.ContextWindow != DefaultAIContextWindow {
		t.Fatalf("context_window = %d, want %d", a.ContextWindow, DefaultAIContextWindow)
	}
	if a.MaxToolCalls != DefaultAIMaxToolCalls {
		t.Fatalf("max_tool_calls = %d, want %d", a.MaxToolCalls, DefaultAIMaxToolCalls)
	}
	if a.SessionTokenLimit != DefaultAISessionTokens {
		t.Fatalf("session_token_limit = %d, want %d", a.SessionTokenLimit, DefaultAISessionTokens)
	}
	if a.DockWidth != DefaultAIDockWidth {
		t.Fatalf("dock_width = %d, want %d", a.DockWidth, DefaultAIDockWidth)
	}
	if !a.DockOpenOn() || !a.LockInputOn() || !a.AutoRestoreOn() {
		t.Fatal("the Agent behavior booleans must default to true")
	}
	if !a.AuditOn() || !a.AuditRedactOn() {
		t.Fatal("the audit booleans must default to true")
	}
	if a.ToolScope != "all" || a.GuardMode != "confirm_all" {
		t.Fatalf("tool_scope/guard_mode = %q/%q, want all/confirm_all", a.ToolScope, a.GuardMode)
	}
	if a.AuditRetentionDays != DefaultAIRetentionDays {
		t.Fatalf("audit_retention_days = %d, want %d", a.AuditRetentionDays, DefaultAIRetentionDays)
	}
}

// A partially written section keeps the explicit values and defaults the rest.
func TestAIDecodePartial(t *testing.T) {
	var cfg Config
	raw := []byte(`{"settings":{"ai":{"provider":"cloud","base_url":"https://api.example.com/v1","model":"m-1","max_tool_calls":12}}}`)
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	cfg.Settings.Normalize()
	a := cfg.Settings.AI

	if a.Provider != AIProviderCloud || a.BaseURL != "https://api.example.com/v1" || a.Model != "m-1" {
		t.Fatalf("explicit values lost: %+v", a)
	}
	if a.MaxToolCalls != 12 {
		t.Fatalf("max_tool_calls = %d, want 12", a.MaxToolCalls)
	}
	if a.DockOpen == nil || !*a.DockOpen {
		t.Fatal("absent dock_open must default to true")
	}
	if a.AuditRetentionDays != DefaultAIRetentionDays {
		t.Fatalf("absent retention = %d, want %d", a.AuditRetentionDays, DefaultAIRetentionDays)
	}
}

// An explicit false survives Normalize and a JSON round trip; nil pointers
// default to true instead of being overwritten.
func TestAIExplicitFalseSurvives(t *testing.T) {
	var cfg Config
	raw := []byte(`{"settings":{"ai":{"dock_open":false,"lock_input":false,"audit_enabled":false,"audit_redact":false}}}`)
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	cfg.Settings.Normalize()
	a := cfg.Settings.AI
	if a.DockOpenOn() || a.LockInputOn() || a.AuditOn() || a.AuditRedactOn() {
		t.Fatal("explicit false must not be replaced by the default true")
	}
	if a.AutoRestoreOn() != true {
		t.Fatal("auto_restore was not set, so it stays true")
	}

	out, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{`"dock_open":false`, `"lock_input":false`, `"audit_enabled":false`, `"audit_redact":false`} {
		if !strings.Contains(string(out), key) {
			t.Fatalf("encoded config lacks %s: %s", key, out)
		}
	}

	var back Config
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	back.Settings.Normalize()
	if back.Settings.AI.DockOpenOn() {
		t.Fatal("dock_open=false did not survive the round trip")
	}
}

// Out-of-range numbers clamp instead of breaking the page.
func TestAINormalizeClamps(t *testing.T) {
	a := AISettings{DockWidth: 100, MaxToolCalls: 900, SessionTokenLimit: 10, AuditRetentionDays: 90_000}
	a.Normalize()
	if a.DockWidth != AIDockWidthMin {
		t.Fatalf("dock_width = %d, want %d", a.DockWidth, AIDockWidthMin)
	}
	if a.MaxToolCalls != 200 {
		t.Fatalf("max_tool_calls = %d, want 200", a.MaxToolCalls)
	}
	if a.SessionTokenLimit != 1_000 {
		t.Fatalf("session_token_limit = %d, want 1000", a.SessionTokenLimit)
	}
	if a.AuditRetentionDays != 3_650 {
		t.Fatalf("audit_retention_days = %d, want 3650", a.AuditRetentionDays)
	}
}

// Validate catches what Normalize cannot repair: a cloud provider with no
// endpoint, and enums that arrived from a hand edit.
func TestAIValidate(t *testing.T) {
	cases := []struct {
		name    string
		ai      AISettings
		wantErr bool
	}{
		{"defaults are valid", AISettings{Provider: AIProviderLocal, BaseURL: DefaultAIBaseURL,
			Model: DefaultAIModel, Reasoning: "high", APIKeyEnv: DefaultAIKeyEnv,
			MaxToolCalls: 40, SessionTokenLimit: 200_000, DockWidth: 340,
			ToolScope: "all", GuardMode: "confirm_all"}, false},
		{"cloud needs a url", AISettings{Provider: AIProviderCloud, Model: DefaultAIModel,
			Reasoning: "high", MaxToolCalls: 40, SessionTokenLimit: 200_000, DockWidth: 340,
			ToolScope: "all", GuardMode: "confirm_all"}, true},
		{"bad reasoning", AISettings{Provider: AIProviderLocal, BaseURL: DefaultAIBaseURL,
			Model: DefaultAIModel, Reasoning: "fast", MaxToolCalls: 40, SessionTokenLimit: 200_000,
			DockWidth: 340, ToolScope: "all", GuardMode: "confirm_all"}, true},
		{"bad guard mode", AISettings{Provider: AIProviderLocal, BaseURL: DefaultAIBaseURL,
			Model: DefaultAIModel, Reasoning: "high", MaxToolCalls: 40, SessionTokenLimit: 200_000,
			DockWidth: 340, ToolScope: "all", GuardMode: "open"}, true},
		{"tools out of range", AISettings{Provider: AIProviderLocal, BaseURL: DefaultAIBaseURL,
			Model: DefaultAIModel, Reasoning: "high", MaxToolCalls: 0, SessionTokenLimit: 200_000,
			DockWidth: 340, ToolScope: "all", GuardMode: "confirm_all"}, true},
	}
	for _, tc := range cases {
		err := tc.ai.Validate()
		if tc.wantErr && err == nil {
			t.Fatalf("%s: expected an error, got nil", tc.name)
		}
		if !tc.wantErr && err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.name, err)
		}
	}
}

// Normalize is idempotent: a second pass changes nothing.
func TestAINormalizeIdempotent(t *testing.T) {
	var a AISettings
	a.Normalize()
	first := a
	a.Normalize()
	if *first.DockOpen != *a.DockOpen || first.DockWidth != a.DockWidth ||
		first.MaxToolCalls != a.MaxToolCalls || *first.AuditEnabled != *a.AuditEnabled {
		t.Fatalf("second Normalize changed the values: %+v -> %+v", first, a)
	}
}
