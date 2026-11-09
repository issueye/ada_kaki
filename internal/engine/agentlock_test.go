package engine

import (
	"strings"
	"testing"
	"time"

	"adakaki/internal/shells"
)

// TestAgentLock checks the mask's engine side: the human's typing is
// dropped while an agent operates the session, and goes through again
// once it ends. The agent's own Send never stops.
func TestAgentLock(t *testing.T) {
	m := NewManager()
	defer m.CloseAll()

	s, err := m.CreateLocal(shells.Cmd, "", "")
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, 10*time.Second, func() bool {
		return s.State() == Running && s.Text() != ""
	})

	s.SetAgentLock(true, "run_command")
	if on, tool := s.AgentLock(); !on || tool != "run_command" {
		t.Fatalf("AgentLock = %v, %q", on, tool)
	}

	// The human's typing is dropped.
	s.UserSend("echo human_typed\r\n")

	// The agent's typing goes through.
	s.Send("echo agent_typed\r\n")

	waitFor(t, 10*time.Second, func() bool {
		return strings.Contains(s.Text(), "agent_typed")
	})
	if strings.Contains(s.Text(), "human_typed") {
		t.Error("UserSend leaked through the lock")
	}

	// The lock lifts: the human types again.
	s.SetAgentLock(false, "")
	s.UserSend("echo human_again\r\n")
	waitFor(t, 10*time.Second, func() bool {
		return strings.Contains(s.Text(), "human_again")
	})
	s.Close()
}
