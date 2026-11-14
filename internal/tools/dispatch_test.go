package tools_test

import (
	"encoding/json"
	"strings"
	"testing"

	"adakaki/internal/engine"
	"adakaki/internal/store"
	"adakaki/internal/tools"
)

func dispatchBackend(t *testing.T) *tools.Backend {
	t.Helper()
	mgr := engine.NewManager()
	t.Cleanup(mgr.CloseAll)
	return tools.NewBackend(mgr, &store.Config{})
}

// Call reaches the same backend methods the MCP transport reaches.
func TestCallDispatch(t *testing.T) {
	b := dispatchBackend(t)

	out, err := b.Call("list_sessions", `{}`)
	if err != nil {
		t.Fatalf("list_sessions: %v", err)
	}
	var sessions []map[string]any
	if err := json.Unmarshal([]byte(out), &sessions); err != nil {
		t.Fatalf("list_sessions output: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("fresh engine lists %d sessions", len(sessions))
	}

	out, err = b.Call("list_shells", "")
	if err != nil {
		t.Fatalf("list_shells: %v", err)
	}
	if !strings.Contains(out, "id") {
		t.Fatalf("list_shells output lacks ids: %s", out)
	}
}

func TestCallUnknownTool(t *testing.T) {
	b := dispatchBackend(t)
	if _, err := b.Call("teleport", `{}`); err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("want an unknown-tool error, got %v", err)
	}
}

func TestCallBadArguments(t *testing.T) {
	b := dispatchBackend(t)
	if _, err := b.Call("list_sessions", `{not json}`); err == nil ||
		!strings.Contains(err.Error(), "invalid arguments") {
		t.Fatalf("want an invalid-arguments error, got %v", err)
	}
}

// run_command keeps its required command, and the empty command never
// reaches a session.
func TestCallRunCommandRequiresCommand(t *testing.T) {
	b := dispatchBackend(t)
	if _, err := b.Call("run_command", `{"command":""}`); err == nil ||
		!strings.Contains(err.Error(), "command is required") {
		t.Fatalf("want a command-required error, got %v", err)
	}
}

func TestCallUnknownSession(t *testing.T) {
	b := dispatchBackend(t)
	if _, err := b.Call("screen_session", `{"id":"missing"}`); err == nil {
		t.Fatal("screen_session should reject unknown ids")
	}
	if _, err := b.Call("resize_session", `{"id":"missing","cols":80,"rows":24}`); err == nil {
		t.Fatal("resize_session should reject unknown ids")
	}
}
