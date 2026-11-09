package engine

import (
	"os"
	"testing"
	"time"

	"adakaki/internal/shells"
)

// TestRestoreSpecs checks that sessions describe themselves for the
// restore list: local shells with their command, SSH with its profile,
// profileless SSH left out.
func TestRestoreSpecs(t *testing.T) {
	m := NewManager()
	defer m.CloseAll()

	s1, err := m.CreateLocal(shells.Custom, `C:\tmp`, "cmd.exe /k chcp 65001 >nul")
	if err != nil {
		t.Fatal(err)
	}
	s2, err := m.CreateLocal(shells.Cmd, "", "")
	if err != nil {
		t.Fatal(err)
	}
	s3, err := m.CreateSSH("", func(s *Session) (Conn, string, error) {
		return newFakeSSHConn("BANNER\r\n"), "u@h", nil
	})
	if err != nil {
		t.Fatal(err)
	}

	specs := m.RestoreSpecs()
	// Two local sessions; the profileless SSH one cannot re-auth after a
	// restart, so the list leaves it out.
	if len(specs) != 2 {
		t.Fatalf("%d specs; want 2", len(specs))
	}
	if specs[0].Kind != "local" || specs[0].Shell != string(shells.Custom) ||
		specs[0].Cwd != `C:\tmp` || specs[0].Custom != "cmd.exe /k chcp 65001 >nul" {
		t.Errorf("spec[0] = %+v", specs[0])
	}
	home, _ := os.UserHomeDir() // an empty cwd becomes the user's home
	if specs[1].Kind != "local" || specs[1].Shell != string(shells.Cmd) || specs[1].Cwd != home {
		t.Errorf("spec[1] = %+v; want cmd in %q", specs[1], home)
	}
	// A profileless SSH session cannot re-auth after a restart: skipped.
	_ = s3
	// After closing s1 and s2 the list follows.
	s1.Close()
	s2.Close()
	waitFor(t, 2*time.Second, func() bool { return len(m.List()) == 1 })
	specs = m.RestoreSpecs()
	if len(specs) != 0 {
		t.Errorf("profileless ssh spec leaked: %+v", specs)
	}
	s3.Close()
	waitFor(t, 2*time.Second, func() bool { return len(m.List()) == 0 })
}
