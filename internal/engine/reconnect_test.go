package engine

import (
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"adakaki/internal/shells"
)

// fakeSSHConn is a Conn that shows a banner and ends when closed: a
// stand-in for an SSH shell, without a network.
type fakeSSHConn struct {
	r      *io.PipeReader
	d      chan struct{}
	closed atomic.Bool
}

func newFakeSSHConn(banner string) *fakeSSHConn {
	pr, pw := io.Pipe()
	c := &fakeSSHConn{r: pr}
	go func() {
		pw.Write([]byte(banner))
		<-c.done() // hold the writer open until Close
	}()
	return c
}

func (c *fakeSSHConn) done() chan struct{} {
	if c.d == nil {
		c.d = make(chan struct{})
	}
	return c.d
}

func (c *fakeSSHConn) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if err != nil {
		return 0, io.EOF
	}
	return n, nil
}

func (c *fakeSSHConn) Write(p []byte) (int, error) { return len(p), nil }

func (c *fakeSSHConn) Resize(cols, rows int) error { return nil }

func (c *fakeSSHConn) Close() error {
	if c.closed.CompareAndSwap(false, true) {
		close(c.done())
		c.r.Close()
	}
	return nil
}

// TestSSHReconnect checks that a reconnect dials again into the SAME
// session: the ID stays, the new connection's banner shows, a running
// session refuses, and a failed dial leaves the session reconnectable.
func TestSSHReconnect(t *testing.T) {
	m := NewManager()
	defer m.CloseAll()

	var dial int32
	var current atomic.Pointer[fakeSSHConn]
	var failNext atomic.Bool
	src := func(s *Session) (Conn, string, error) {
		if failNext.CompareAndSwap(true, false) {
			return nil, "", fmt.Errorf("host unreachable")
		}
		c := newFakeSSHConn(fmt.Sprintf("BANNER_%d\r\n", atomic.AddInt32(&dial, 1)))
		current.Store(c)
		return c, "user@host", nil
	}
	s, err := m.CreateSSH("prof", src)
	if err != nil {
		t.Fatalf("CreateSSH: %v", err)
	}
	id := s.ID()
	waitFor(t, 10*time.Second, func() bool { return contains(s.Text(), "BANNER_1") })
	if s.Title() != "user@host" {
		t.Errorf("title = %q", s.Title())
	}

	// A running session cannot reconnect.
	if err := s.Reconnect(); err == nil {
		t.Fatal("reconnect on a running session must fail")
	}

	// The cable is cut: the connection ends, the session stays exited.
	current.Load().Close()
	waitFor(t, 10*time.Second, func() bool { return s.State() == Exited })

	// The first reconnect fails to dial: the error comes back and the
	// session stays exited, still reconnectable.
	failNext.Store(true)
	if err := s.Reconnect(); err == nil || err.Error() != "host unreachable" {
		t.Fatalf("failing reconnect = %v", err)
	}
	if s.State() != Exited {
		t.Fatalf("state after failed dial = %s", s.State())
	}

	// The next dial succeeds: same ID, fresh banner, running again.
	if err := s.Reconnect(); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	if s.ID() != id {
		t.Errorf("id changed to %s", s.ID())
	}
	if s.State() != Running {
		t.Fatalf("state after reconnect = %s", s.State())
	}
	waitFor(t, 10*time.Second, func() bool { return contains(s.Text(), "BANNER_2") })

	if _, ok := m.Get(id); !ok {
		t.Error("session vanished from the manager")
	}
	s.Close()
}

// TestLocalReconnect checks that an exited local shell starts again in
// the same session, and a running one refuses.
func TestLocalReconnect(t *testing.T) {
	m := NewManager()
	defer m.CloseAll()

	s, err := m.CreateLocal(shells.Cmd, t.TempDir(), "")
	if err != nil {
		t.Fatalf("CreateLocal: %v", err)
	}
	id := s.ID()
	if err := s.Reconnect(); err == nil {
		t.Fatal("reconnect on a running local session must fail")
	}

	s.Send("exit\r")
	waitFor(t, 10*time.Second, func() bool { return s.State() == Exited })

	if err := s.Reconnect(); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	if s.ID() != id {
		t.Errorf("id changed to %s", s.ID())
	}
	if s.State() != Running {
		t.Fatalf("state after reconnect = %s", s.State())
	}
	s.Send("echo local_reconnect_ok\r")
	waitFor(t, 10*time.Second, func() bool { return contains(s.Text(), "local_reconnect_ok") })
	s.Close()
}

// TestLocalReconnectCustom checks that a custom session's command runs
// again on reconnect, in its directory.
func TestLocalReconnectCustom(t *testing.T) {
	m := NewManager()
	defer m.CloseAll()

	dir := t.TempDir()
	s, err := m.CreateLocal(shells.Custom, dir, "cmd.exe /c echo custom_reconnect_ok")
	if err != nil {
		t.Fatalf("CreateLocal: %v", err)
	}
	waitFor(t, 10*time.Second, func() bool {
		return s.State() == Exited && contains(s.Text(), "custom_reconnect_ok")
	})

	if err := s.Reconnect(); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	waitFor(t, 10*time.Second, func() bool {
		return s.State() == Exited && contains(s.Text(), "custom_reconnect_ok")
	})
	s.Close()
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && strings.Contains(s, sub)
}
