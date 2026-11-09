package engine

import (
	"strings"
	"testing"
	"time"

	"adakaki/internal/shells"
	"github.com/egoist/mygo/plugins/terminal"
)

// TestThemeAndFont checks the manager takes a color scheme and a font
// and hands both to the sessions it creates afterwards.
func TestThemeAndFont(t *testing.T) {
	m := NewManager()
	defer m.CloseAll()

	// An unknown scheme is refused, a shipped one is taken.
	if err := m.SetTheme("no-such-theme-xyz"); err == nil {
		t.Error("an unknown theme name must be refused")
	}
	if err := m.SetTheme("Dracula"); err != nil {
		t.Fatalf("SetTheme Dracula: %v", err)
	}
	m.SetFont(terminal.Font{Family: "Consolas", Size: 15})

	// Sessions still open, and the screen works, under the new theme.
	s, err := m.CreateLocal(shells.Custom, "", "cmd.exe /c echo themed_ok")
	if err != nil {
		t.Fatalf("CreateLocal: %v", err)
	}
	waitFor(t, 10*time.Second, func() bool {
		return s.State() == Exited && strings.Contains(s.Text(), "themed_ok")
	})

	// Back to following the window.
	if err := m.SetTheme(""); err != nil {
		t.Fatalf("SetTheme(\"\": %v", err)
	}
	s2, err := m.CreateLocal(shells.Custom, "", "cmd.exe /c echo plain_ok")
	if err != nil {
		t.Fatalf("CreateLocal: %v", err)
	}
	waitFor(t, 10*time.Second, func() bool {
		return contains(s2.Text(), "plain_ok")
	})
}
