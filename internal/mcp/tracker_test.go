package mcp

import (
	"sync"
	"testing"

	"adakaki/internal/engine"
	"adakaki/internal/shells"
	"adakaki/internal/store"
	"adakaki/internal/tools"
)

// TestAgentTrackerCalled checks that a tool call marks the session it
// operates for its whole duration, and releases it when done.
func TestAgentTrackerCalled(t *testing.T) {
	mgr := engine.NewManager()
	defer mgr.CloseAll()
	b := tools.NewBackend(mgr, &store.Config{})

	var mu sync.Mutex
	var calls []string
	b.SetAgentTracker(func(id, tool string) func() {
		mu.Lock()
		calls = append(calls, "begin:"+id+":"+tool)
		mu.Unlock()
		return func() {
			mu.Lock()
			calls = append(calls, "release:"+id+":"+tool)
			mu.Unlock()
		}
	})

	s, err := mgr.CreateLocal(shells.Cmd, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.WriteSession(s.ID(), "echo hi\r\n"); err != nil {
		t.Fatalf("WriteSession: %v", err)
	}
	if _, err := b.ScreenSession(s.ID()); err != nil {
		t.Fatalf("ScreenSession: %v", err)
	}

	mu.Lock()
	got := append([]string(nil), calls...)
	mu.Unlock()
	want := []string{
		"begin:" + s.ID() + ":write_session",
		"release:" + s.ID() + ":write_session",
		"begin:" + s.ID() + ":screen_session",
		"release:" + s.ID() + ":screen_session",
	}
	if len(got) != len(want) {
		t.Fatalf("calls = %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("calls[%d] = %q; want %q", i, got[i], want[i])
		}
	}

	// A nil tracker (stdio mode) never blocks anything.
	b2 := tools.NewBackend(mgr, &store.Config{})
	if _, err := b2.WriteSession(s.ID(), "echo hi\r\n"); err != nil {
		t.Errorf("WriteSession without tracker: %v", err)
	}
}
