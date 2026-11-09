package engine

import (
	"strings"
	"sync"
	"testing"
	"time"

	"adakaki/internal/shells"
)

// waitFor polls until fn is true, for tests.
func waitFor(t *testing.T, d time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out")
}

func TestCreateLocalEcho(t *testing.T) {
	m := NewManager()
	defer m.CloseAll()

	// A one-shot command as a custom session: it echoes and exits.
	s, err := m.CreateLocal(shells.Custom, t.TempDir(), "cmd.exe /c echo engine_ok")
	if err != nil {
		t.Fatalf("CreateLocal: %v", err)
	}
	if s.Kind() != Local || s.Shell() != shells.Custom {
		t.Errorf("session = %v/%v", s.Kind(), s.Shell())
	}
	if s.Title() != "cmd.exe" {
		t.Errorf("Title = %q; want cmd.exe", s.Title())
	}
	if got, ok := m.Get(s.ID()); !ok || got != s {
		t.Error("Get did not return the session")
	}
	waitFor(t, 10*time.Second, func() bool { return s.State() == Exited })
	if code := s.ExitCode(); code != 0 {
		t.Errorf("ExitCode = %d; want 0", code)
	}
	if text := s.Text(); !strings.Contains(text, "engine_ok") {
		t.Errorf("Text %q does not contain the echo", text)
	}
}

func TestEventsAndRemove(t *testing.T) {
	m := NewManager()
	defer m.CloseAll()

	var mu sync.Mutex
	var kinds []EventKind
	cancel := m.Subscribe(func(ev Event) {
		mu.Lock()
		kinds = append(kinds, ev.Kind)
		mu.Unlock()
	})

	s, err := m.CreateLocal(shells.Custom, "", "cmd.exe /c echo bye")
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, 10*time.Second, func() bool { return s.State() == Exited })

	mu.Lock()
	got := append([]EventKind(nil), kinds...)
	mu.Unlock()
	if len(got) < 2 || got[0] != EventCreated || got[len(got)-1] != EventExited {
		t.Errorf("events = %v; want created…exited", got)
	}

	cancel()
	n := len(m.List())
	s.Close()
	waitFor(t, 2*time.Second, func() bool { return len(m.List()) == n-1 })
	mu.Lock()
	after := append([]EventKind(nil), kinds...)
	mu.Unlock()
	if len(after) != len(got) {
		t.Errorf("events after unsubscribe grew: %v", after)
	}
}

func TestSendInteractive(t *testing.T) {
	m := NewManager()
	defer m.CloseAll()

	s, err := m.CreateLocal(shells.Cmd, "", "")
	if err != nil {
		t.Fatal(err)
	}
	s.Send("echo sent_ok\r\n")
	waitFor(t, 10*time.Second, func() bool {
		return strings.Contains(s.Text(), "sent_ok")
	})
	s.Close()
	waitFor(t, 2*time.Second, func() bool { return s.State() == Exited })
}

func TestConcurrentSessions(t *testing.T) {
	m := NewManager()
	defer m.CloseAll()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := m.CreateLocal(shells.Cmd, "", "")
			if err != nil {
				t.Errorf("CreateLocal: %v", err)
				return
			}
			s.Close()
		}()
	}
	wg.Wait()
	if n := len(m.List()); n != 0 {
		t.Errorf("%d sessions left; want 0", n)
	}
}
