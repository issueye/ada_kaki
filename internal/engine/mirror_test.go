package engine

import (
	"strings"
	"testing"
	"time"

	"adakaki/internal/shells"
)

// TestMirrorView checks that a second view of a session shows what the
// session shows and takes input for it.
func TestMirrorView(t *testing.T) {
	m := NewManager()
	defer m.CloseAll()

	s, err := m.CreateLocal(shells.Cmd, "", "")
	if err != nil {
		t.Fatal(err)
	}
	s.Send("echo mirror_ok\r\n")

	mv, err := s.OpenMirror()
	if err != nil {
		t.Fatalf("OpenMirror: %v", err)
	}
	defer mv.Close()

	// The session's output reaches the mirror's screen.
	waitFor(t, 15*time.Second, func() bool {
		return strings.Contains(mv.Terminal().Text(), "mirror_ok")
	})

	// What is typed on the mirror end drives the session.
	mv.Terminal().Send([]byte("echo from_mirror\r\n"))
	waitFor(t, 15*time.Second, func() bool {
		return strings.Contains(s.Text(), "from_mirror")
	})
	// And the session's echo of it comes back to the mirror.
	waitFor(t, 15*time.Second, func() bool {
		return strings.Contains(mv.Terminal().Text(), "from_mirror")
	})

	// Sizes track the session's.
	cols, rows := s.Size()
	if c, r := mv.Terminal().Size(); c != cols || r != rows {
		t.Errorf("mirror size = %dx%d; want %dx%d", c, r, cols, rows)
	}

	// A second mirror of the same session is fine.
	mv2, err := s.OpenMirror()
	if err != nil {
		t.Fatalf("second OpenMirror: %v", err)
	}
	mv2.Close()

	s.Close()
}
