package tools_test

import (
	"encoding/json"
	"testing"

	"adakaki/internal/tools"
)

// The catalog the agent offers the model, in the order the MCP transport
// publishes. internal/mcp pins these names against its own catalog.
func TestSpecsCatalog(t *testing.T) {
	want := []string{
		"list_shells", "list_sessions", "create_session", "ssh_connect",
		"list_ssh_profiles", "write_session", "read_session", "screen_session",
		"resize_session", "close_session", "reconnect_session", "run_command",
	}
	specs := tools.Specs()
	if len(specs) != len(want) {
		t.Fatalf("spec count = %d, want %d", len(specs), len(want))
	}
	for i, name := range want {
		if specs[i].Name != name {
			t.Fatalf("spec %d = %q, want %q", i, specs[i].Name, name)
		}
		if specs[i].Description == "" {
			t.Fatalf("%q has no description", name)
		}
		var schema map[string]any
		if err := json.Unmarshal(specs[i].Schema, &schema); err != nil {
			t.Fatalf("%q schema is not JSON: %v", name, err)
		}
		if schema["type"] != "object" {
			t.Fatalf("%q schema type = %v, want object", name, schema["type"])
		}
	}
}

// The workhorse keeps its argument contract: the model is told about id,
// shell, cwd, command and the two timing knobs, with command required.
func TestRunCommandSpec(t *testing.T) {
	var spec tools.Spec
	for _, s := range tools.Specs() {
		if s.Name == "run_command" {
			spec = s
		}
	}
	if spec.Name == "" {
		t.Fatal("run_command is missing")
	}
	var schema struct {
		Type       string                     `json:"type"`
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(spec.Schema, &schema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	for _, key := range []string{"id", "shell", "cwd", "command", "timeout_ms", "quiet_ms"} {
		if _, ok := schema.Properties[key]; !ok {
			t.Fatalf("run_command lost %q", key)
		}
	}
	if len(schema.Required) != 1 || schema.Required[0] != "command" {
		t.Fatalf("required = %v, want [command]", schema.Required)
	}
}

// CommandOf feeds the guard and the audit the line a call would run.
func TestCommandOf(t *testing.T) {
	if got := tools.CommandOf("run_command", `{"command":"rm -rf x"}`); got != "rm -rf x" {
		t.Fatalf("CommandOf(run_command) = %q", got)
	}
	if got := tools.CommandOf("write_session", `{"data":"npm run build\r"}`); got != "npm run build\r" {
		t.Fatalf("CommandOf(write_session) = %q", got)
	}
	for _, name := range []string{"list_sessions", "read_session", "resize_session", "unknown"} {
		if got := tools.CommandOf(name, `{"id":"s1"}`); got != "" {
			t.Fatalf("CommandOf(%s) = %q, want empty", name, got)
		}
	}
}
