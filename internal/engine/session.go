// Package engine is the console's session engine: it wraps the terminal
// plugin's Terminal as a Session with a state machine and an event bus,
// shared by the GUI and the MCP server.
package engine

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"adakaki/internal/procjob"
	"adakaki/internal/shells"
	"github.com/egoist/mygo/plugins/terminal"
)

// Kind tells a local shell session from an SSH one.
type Kind string

const (
	Local Kind = "local"
	SSH   Kind = "ssh"
)

// State of a session.
type State string

const (
	Starting State = "starting"
	Running  State = "running"
	Exited   State = "exited"
)

// EventKind names what happened to a session.
type EventKind string

const (
	EventCreated     EventKind = "created"
	EventExited      EventKind = "exited"
	EventTitle       EventKind = "title"
	EventClosed      EventKind = "closed"
	EventReconnected EventKind = "reconnected"
)

// Event is one thing that happened, broadcast to subscribers.
type Event struct {
	Kind     EventKind
	ID       string
	Kind2    Kind // session's kind
	Title    string
	ExitCode int
	Err      string // why the session ended, when not cleanly
}

// Session is one terminal session: the plugin's Terminal plus metadata
// and state. Its methods are safe from any goroutine.
type Session struct {
	id      string
	kind    Kind
	shellID shells.ID
	profile string

	mu       sync.Mutex
	title    string
	state    State
	exitCode int
	exitErr  string
	term     *terminal.Terminal
	rootPID  int    // a local shell's process, for a force close
	exeBase  string // its executable's name, to ignore a bare path title
	agentMux sync.Mutex
	agent    bool   // an AI agent tool call is operating this session
	agentDo  string // the tool now running

	source SSHSource // how to dial this SSH session again; nil for local

	cwd    string // where a local shell started, for the restore list
	custom string // a custom session's command line, likewise

	reconnecting bool
	search       terminal.SearchStatus

	mgr *Manager
}

func (s *Session) ID() string       { return s.id }
func (s *Session) Kind() Kind       { return s.kind }
func (s *Session) Shell() shells.ID { return s.shellID }
func (s *Session) Profile() string  { return s.profile }

// Cwd returns where a local session started, "" for an SSH one.
func (s *Session) Cwd() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cwd
}

// Title returns the session's title: the shell's display name until the
// program sets its own (OSC 0/2).
func (s *Session) Title() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.title
}

func (s *Session) setTitle(t string) {
	s.mu.Lock()
	s.title = t
	s.mu.Unlock()
	s.mgr.broadcast(Event{Kind: EventTitle, ID: s.id, Kind2: s.kind, Title: t})
}

// State returns the session's state.
func (s *Session) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// ExitCode returns the program's exit code, meaningful once Exited.
func (s *Session) ExitCode() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exitCode
}

// ExitErr returns why the session ended, when not cleanly (SSH cut off).
func (s *Session) ExitErr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exitErr
}

// Size returns the terminal's size in cells.
func (s *Session) Size() (cols, rows int) {
	if t := s.terminal(); t != nil {
		return t.Size()
	}
	return 80, 24
}

// Terminal returns the session's terminal, for the UI to show in a view;
// nil once the session is closed.
func (s *Session) Terminal() *terminal.Terminal {
	return s.terminal()
}

// ---- search ----

// Search searches the screen and scrollback; "" ends the search. A view
// drives the finding, so the status moves as the session's terminal
// repaints.
func (s *Session) Search(text string) {
	if t := s.terminal(); t != nil {
		t.Search(text)
	}
}

// SearchNext selects the match toward older output; SearchPrevious the
// other way.
func (s *Session) SearchNext() {
	if t := s.terminal(); t != nil {
		t.SearchNext()
	}
}

func (s *Session) SearchPrevious() {
	if t := s.terminal(); t != nil {
		t.SearchPrevious()
	}
}

// SearchStatus is how the session's search goes: the find bar shows it.
func (s *Session) SearchStatus() terminal.SearchStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.search
}

// onSearchStatus is the terminal's OnSearch callback (main thread).
func (s *Session) onSearchStatus(st terminal.SearchStatus) {
	s.mu.Lock()
	s.search = st
	s.mu.Unlock()
}

// ---- font ----

// SetFont applies a font to the session's terminal, live.
func (s *Session) SetFont(f terminal.Font) {
	if t := s.terminal(); t != nil {
		t.SetFont(f)
	}
}

// ApplyTheme switches the session's color scheme live; nil follows the
// window's appearance.
func (s *Session) ApplyTheme(theme, dark *terminal.Theme) {
	if t := s.terminal(); t != nil {
		t.SetTheme(theme, dark)
		t.UseTheme(theme) // viewless terminals answer color queries with it
	}
}

// Send writes data to the program, as typed. The agent's path: MCP tools
// go through here whatever the lock says.
func (s *Session) Send(data string) {
	if t := s.terminal(); t != nil {
		t.Send([]byte(data))
	}
}

// UserSend writes what a human typed. While an agent operates the
// session it is dropped, quietly: the human can watch, not disturb.
func (s *Session) UserSend(data string) {
	if s.agentLocked() {
		return
	}
	s.Send(data)
}

// SetAgentLock marks the session as under an agent tool call, with the
// tool's name for the mask to show.
func (s *Session) SetAgentLock(on bool, tool string) {
	s.agentMux.Lock()
	s.agent, s.agentDo = on, tool
	s.agentMux.Unlock()
}

// AgentLock reports whether an agent is operating the session, and the
// tool that is running.
func (s *Session) AgentLock() (bool, string) {
	s.agentMux.Lock()
	defer s.agentMux.Unlock()
	return s.agent, s.agentDo
}

func (s *Session) agentLocked() bool {
	s.agentMux.Lock()
	defer s.agentMux.Unlock()
	return s.agent
}

// Text returns the screen and scrollback as plain text.
func (s *Session) Text() string {
	if t := s.terminal(); t != nil {
		return t.Text()
	}
	return ""
}

// Snapshot returns the screen as escape sequences, to replay elsewhere.
func (s *Session) Snapshot() []byte {
	if t := s.terminal(); t != nil {
		return t.Snapshot()
	}
	return nil
}

// Resize sets the size of a terminal no view shows.
func (s *Session) Resize(cols, rows int) {
	if t := s.terminal(); t != nil {
		t.Resize(cols, rows)
	}
}

// NewMirror returns a mirror of the session's terminal, for a second view
// (a popped-out window).
func (s *Session) NewMirror() (*terminal.Mirror, error) {
	if t := s.terminal(); t != nil {
		return t.NewMirror()
	}
	return nil, fmt.Errorf("engine: session %s is closed", s.id)
}

func (s *Session) terminal() *terminal.Terminal {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.term
}

// Close closes the session: the terminal's program ends, whatever it is,
// and the manager drops the session.
func (s *Session) Close() {
	s.mgr.remove(s.id)
}

// ForceClose ends a local session's whole process tree at once: its shell,
// the command running in it, and whatever that started, without the hangup
// and the wait Close allows. An SSH session, which has no local tree, is
// closed by its connection as Close does.
func (s *Session) ForceClose() error {
	s.mu.Lock()
	pid := s.rootPID
	s.mu.Unlock()
	if pid != 0 {
		if err := procjob.KillTree(pid); err != nil {
			s.mgr.remove(s.id)
			return err
		}
	}
	s.mgr.remove(s.id)
	return nil
}

// onExit is the terminal's OnExit callback.
func (s *Session) onExit(code int) {
	s.mu.Lock()
	if s.state == Exited {
		s.mu.Unlock()
		return
	}
	s.state = Exited
	s.exitCode = code
	s.mu.Unlock()
	s.mgr.broadcast(Event{Kind: EventExited, ID: s.id, Kind2: s.kind, Title: s.Title(), ExitCode: code})
}

// Reconnect restarts an exited session in place: an SSH session dials its
// connection again; a local session starts its shell or custom command
// again, in the same directory. The ID, pane, tab and title stay.
func (s *Session) Reconnect() error {
	if s.kind != SSH && s.kind != Local {
		return fmt.Errorf("engine: session %s cannot reconnect", s.id)
	}
	if s.kind == SSH && s.source == nil {
		return fmt.Errorf("engine: session %s cannot reconnect", s.id)
	}
	s.mu.Lock()
	if s.state == Running {
		s.mu.Unlock()
		return fmt.Errorf("engine: session %s is still running", s.id)
	}
	if s.reconnecting {
		s.mu.Unlock()
		return fmt.Errorf("engine: session %s is already reconnecting", s.id)
	}
	s.reconnecting = true
	s.state = Starting
	s.mu.Unlock()
	s.mgr.broadcast(Event{Kind: EventTitle, ID: s.id, Kind2: s.kind, Title: s.Title()})

	fail := func(err error) error {
		s.mu.Lock()
		s.state = Exited
		s.reconnecting = false
		s.mu.Unlock()
		s.mgr.broadcast(Event{Kind: EventExited, ID: s.id, Kind2: s.kind, Title: s.Title(), Err: err.Error()})
		return err
	}

	var term *terminal.Terminal
	var title string
	pid := 0
	switch s.kind {
	case SSH:
		conn, t, err := s.source(s)
		if err != nil {
			return fail(err)
		}
		term, err = terminal.New(sshTerminalOpts(conn, s, s.mgr))
		if err != nil {
			_ = conn.Close()
			return fail(fmt.Errorf("engine: reconnecting %s: %w", s.id, err))
		}
		title = t
	case Local:
		cmd, _, dir, err := s.mgr.localCommand(s.shellID, s.cwd, s.custom)
		if err != nil {
			return fail(err)
		}
		if len(cmd) > 0 {
			s.mu.Lock()
			s.exeBase = filepath.Base(cmd[0])
			s.mu.Unlock()
		}
		term, pid, err = s.mgr.launchLocal(s, cmd, dir, nil)
		if err != nil {
			return fail(fmt.Errorf("engine: reconnecting %s: %w", s.id, err))
		}
	}

	s.mu.Lock()
	old := s.term
	s.term = term
	s.rootPID = pid
	if s.state == Starting {
		s.state = Running
	}
	s.reconnecting = false
	s.exitCode = 0
	s.exitErr = ""
	if title != "" {
		s.title = title
	}
	s.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	s.mgr.broadcast(Event{Kind: EventReconnected, ID: s.id, Kind2: s.kind, Title: s.Title()})
	return nil
}

// setTitleFrom is the terminal's OnTitle callback. A shell reports its
// executable's path as its first title; that is not a name worth showing,
// so the shell's display name stays until the program sets a real title.
func (s *Session) setTitleFrom(title string) {
	if title == "" {
		return
	}
	s.mu.Lock()
	exe := s.exeBase
	s.mu.Unlock()
	if exe != "" && filepath.IsAbs(title) && strings.EqualFold(filepath.Base(title), exe) {
		return
	}
	s.setTitle(title)
}
