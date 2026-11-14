package tools_test

import (
	"encoding/json"
	"testing"

	"adakaki/internal/engine"
	"adakaki/internal/store"
	"adakaki/internal/tools"
)

// The extracted tool layer still answers the two read-only tools without a
// session, which is what the stdio mode does first.
func TestBackendReadOnlyTools(t *testing.T) {
	mgr := engine.NewManager()
	defer mgr.CloseAll()
	b := tools.NewBackend(mgr, &store.Config{})

	out, err := b.ListShells()
	if err != nil {
		t.Fatalf("ListShells: %v", err)
	}
	var shells []map[string]any
	if err := json.Unmarshal([]byte(out), &shells); err != nil {
		t.Fatalf("ListShells is not JSON: %v", err)
	}
	if len(shells) == 0 {
		t.Fatal("ListShells returned nothing; at least one shell should be detected")
	}
	for _, sh := range shells {
		if _, ok := sh["id"]; !ok {
			t.Fatalf("a shell entry lacks id: %v", sh)
		}
		if _, ok := sh["name"]; !ok {
			t.Fatalf("a shell entry lacks name: %v", sh)
		}
	}

	out, err = b.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	var sessions []map[string]any
	if err := json.Unmarshal([]byte(out), &sessions); err != nil {
		t.Fatalf("ListSessions is not JSON: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("a fresh engine should list no sessions, got %d", len(sessions))
	}
}

// Unknown session ids are errors, not empty successes.
func TestBackendUnknownSession(t *testing.T) {
	mgr := engine.NewManager()
	defer mgr.CloseAll()
	b := tools.NewBackend(mgr, &store.Config{})

	if _, err := b.WriteSession("nope", "x"); err == nil {
		t.Fatal("WriteSession should reject unknown ids")
	}
	if _, err := b.ReadSession("nope", 0, 100); err == nil {
		t.Fatal("ReadSession should reject unknown ids")
	}
	if _, err := b.ResizeSession("nope", 80, 24); err == nil {
		t.Fatal("ResizeSession should reject unknown ids")
	}
	if _, err := b.CloseSession("nope"); err == nil {
		t.Fatal("CloseSession should reject unknown ids")
	}
}
