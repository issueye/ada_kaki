package engine

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"adakaki/internal/procjob"
	"adakaki/internal/shells"
	"github.com/egoist/mygo/plugins/terminal"
)

// Manager owns every session and broadcasts their events.
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	order    []string // creation order, for List
	seq      atomic.Int64
	launchMu sync.Mutex      // one shell starts at a time, to learn its PID
	font     terminal.Font   // what new sessions start with
	theme    *terminal.Theme // the color scheme; nil follows the window
	dark     *terminal.Theme // its dark-window variant, with theme

	subsMu sync.Mutex
	subs   map[int64]func(Event)
	subSeq atomic.Int64
}

// NewManager returns an empty manager.
func NewManager() *Manager {
	return &Manager{sessions: map[string]*Session{}, subs: map[int64]func(Event){}}
}

// Subscribe adds a callback for events; it runs on the goroutine that
// fires the event, so it must not block. The returned handle unsubscribes.
func (m *Manager) Subscribe(fn func(Event)) (cancel func()) {
	handle := m.subSeq.Add(1)
	m.subsMu.Lock()
	m.subs[handle] = fn
	m.subsMu.Unlock()
	return func() {
		m.subsMu.Lock()
		delete(m.subs, handle)
		m.subsMu.Unlock()
	}
}

func (m *Manager) broadcast(ev Event) {
	m.subsMu.Lock()
	fns := make([]func(Event), 0, len(m.subs))
	for _, fn := range m.subs {
		fns = append(fns, fn)
	}
	m.subsMu.Unlock()
	for _, fn := range fns {
		fn(ev)
	}
}

// SetFont sets what new sessions start with, and applies it live to the
// ones already running.
func (m *Manager) SetFont(f terminal.Font) {
	m.mu.Lock()
	m.font = f
	m.mu.Unlock()
	for _, s := range m.List() {
		s.SetFont(f)
	}
}

// SetTheme sets the terminal color scheme: a Ghostty theme by name, or
// "" to follow the window's appearance. New sessions take it, running
// ones switch live.
func (m *Manager) SetTheme(name string) error {
	var theme *terminal.Theme
	if name != "" {
		th, err := terminal.GhosttyTheme(name)
		if err != nil {
			return err
		}
		theme = th
	}
	m.mu.Lock()
	m.theme = theme
	m.dark = nil
	m.mu.Unlock()
	for _, s := range m.List() {
		s.ApplyTheme(theme, nil)
	}
	return nil
}

// currentTheme returns the scheme new sessions start with.
func (m *Manager) currentTheme() *terminal.Theme {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.theme
}

// currentFont returns the font new sessions start with.
func (m *Manager) currentFont() terminal.Font {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.font
}

// CreateLocal starts a local shell session of the shell's ID, in cwd (the
// user's home when empty), running custom for shells.Custom.
func (m *Manager) CreateLocal(id shells.ID, cwd, custom string) (*Session, error) {
	cmd, name, dir, err := m.localCommand(id, cwd, custom)
	if err != nil {
		return nil, err
	}

	exeBase := ""
	if len(cmd) > 0 {
		exeBase = filepath.Base(cmd[0])
	}
	s := &Session{
		kind:    Local,
		shellID: id,
		state:   Starting,
		exeBase: exeBase,
		cwd:     dir,
		custom:  custom,
		mgr:     m,
	}
	s.mu.Lock()
	s.title = name
	s.mu.Unlock()
	s.id = fmt.Sprintf("local_%04d", m.seq.Add(1))

	_, pid, err := m.launchLocal(s, cmd, dir, func(t *terminal.Terminal) {
		// Register the session before waiting for its PID: startup
		// events (a title, a one-shot's exit) then come after Created.
		s.mu.Lock()
		s.term = t
		if s.state != Exited {
			s.state = Running
		}
		s.mu.Unlock()
		m.put(s)
	})
	if err != nil {
		return nil, fmt.Errorf("engine: starting %s: %w", name, err)
	}
	s.mu.Lock()
	s.rootPID = pid
	s.mu.Unlock()
	return s, nil
}

// localCommand resolves what a local session runs, where it starts and
// its initial title, checking the shell and the directory exist.
func (m *Manager) localCommand(id shells.ID, cwd, custom string) (cmd []string, name, dir string, err error) {
	if id == shells.Custom {
		cmd = shells.Launch(id, "", custom)
		name = shells.CustomTitle(custom)
	} else {
		sh, ok := shells.Get(id)
		if !ok || !sh.Found {
			return nil, "", "", fmt.Errorf("engine: shell %s is not available", id)
		}
		cmd = shells.Launch(id, sh.Path, custom)
		name = sh.Name
	}
	if len(cmd) == 0 {
		return nil, "", "", fmt.Errorf("engine: no command for shell %s", id)
	}
	if cwd == "" {
		home, herr := os.UserHomeDir()
		if herr == nil {
			cwd = home
		}
	} else if fi, serr := os.Stat(cwd); serr != nil || !fi.IsDir() {
		return nil, "", "", fmt.Errorf("engine: working directory %s does not exist", cwd)
	}
	return cmd, name, cwd, nil
}

// launchLocal starts a local terminal and learns the PID of its shell:
// the shell is a direct child of this app, so a snapshot before and after
// the start names it. The snapshot waits a moment for the terminal's
// asynchronous launch. One start at a time keeps the diff unambiguous.
func (m *Manager) launchLocal(s *Session, cmd []string, dir string,
	register func(*terminal.Terminal)) (*terminal.Terminal, int, error) {
	m.launchMu.Lock()
	defer m.launchMu.Unlock()

	before, err := procjob.DirectChildren()
	if err != nil {
		before = nil // no diff, so no PID: force close falls back to Close
	}
	term, err := terminal.New(m.localTerminalOpts(s, cmd, dir))
	if err != nil {
		return nil, 0, err
	}
	if register != nil {
		register(term)
	}
	if before == nil {
		return term, 0, nil
	}
	exe := ""
	if len(cmd) > 0 {
		exe = filepath.Base(cmd[0])
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if pid := procjob.NewChild(before, exe); pid != 0 {
			return term, pid, nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return term, 0, nil
}

// localTerminalOpts builds the terminal options for a local session: the
// program with its capture hooks, and the current font and theme.
func (m *Manager) localTerminalOpts(s *Session, cmd []string, dir string) terminal.Options {
	opts := terminal.Options{
		Command:  cmd,
		Dir:      dir,
		OnTitle:  s.setTitleFrom,
		OnExit:   s.onExit,
		OnSearch: s.onSearchStatus,
		// Windows Terminal's secondary click: copy the selection, or
		// paste when there is none. The pane's own menu (split, close)
		// lives on the tab and the sidebar.
		RightClick: terminal.RightClickContextMenu,
	}
	if f := m.currentFont(); f.Family != "" || f.Size > 0 {
		opts.Font = f
	}
	opts.Theme = m.currentTheme()
	return opts
}

// CreateSSH is the SSH entry point: source dials the connection (the
// sshc package), returning the stream the terminal reads and writes and
// the session's initial title.
func (m *Manager) CreateSSH(profile string, source SSHSource) (*Session, error) {
	if source == nil {
		return nil, fmt.Errorf("engine: ssh is not available")
	}
	s := &Session{
		kind:    SSH,
		profile: profile,
		state:   Starting,
		mgr:     m,
	}
	s.id = fmt.Sprintf("ssh_%04d", m.seq.Add(1))
	conn, title, err := source(s)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.title = title
	s.source = source
	s.mu.Unlock()

	term, err := terminal.New(sshTerminalOpts(conn, s, m))
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("engine: starting ssh session: %w", err)
	}
	s.mu.Lock()
	s.term = term
	s.state = Running
	s.mu.Unlock()

	m.put(s)
	return s, nil
}

// sshTerminalOpts builds the options of a terminal wired to an SSH
// connection: its end closes the connection and marks the session.
func sshTerminalOpts(conn Conn, s *Session, m *Manager) terminal.Options {
	opts := terminal.Options{
		Conn:       conn,
		OnTitle:    s.setTitleFrom,
		OnSearch:   s.onSearchStatus,
		RightClick: terminal.RightClickContextMenu,
		OnExit: func(code int) {
			_ = conn.Close()
			s.onExit(code)
		},
	}
	opts.Theme = m.currentTheme()
	if f := m.currentFont(); f.Family != "" || f.Size > 0 {
		opts.Font = f
	}
	return opts
}

// RestoreSpec describes a session well enough to open it again after a
// restart: the list the app saves on every change.
type RestoreSpec struct {
	Kind    string `json:"kind"` // "local" | "ssh"
	Shell   string `json:"shell,omitempty"`
	Cwd     string `json:"cwd,omitempty"`
	Custom  string `json:"custom,omitempty"`
	Profile string `json:"profile,omitempty"`
}

// RestoreSpecs describes every session, in creation order; SSH sessions
// without a profile are left out (their password is nowhere to read).
func (m *Manager) RestoreSpecs() []RestoreSpec {
	var out []RestoreSpec
	for _, s := range m.List() {
		switch s.Kind() {
		case Local:
			out = append(out, RestoreSpec{
				Kind:   "local",
				Shell:  string(s.Shell()),
				Cwd:    s.cwd,
				Custom: s.custom,
			})
		case SSH:
			if s.profile != "" {
				out = append(out, RestoreSpec{Kind: "ssh", Profile: s.profile})
			}
		}
	}
	return out
}

// Conn is what a session's terminal talks to when the session is not a
// local process, an SSH shell, say: the plugin reads the terminal's
// output from it and writes what is typed to it, and tells it the view's
// size through its Resize, which the plugin looks up by type.
type Conn interface {
	io.ReadWriteCloser
	Resize(cols, rows int) error
}

// SSHSource dials one SSH connection for a session.
type SSHSource func(s *Session) (Conn, string, error)

func (m *Manager) put(s *Session) {
	m.mu.Lock()
	m.sessions[s.id] = s
	m.order = append(m.order, s.id)
	m.mu.Unlock()
	m.broadcast(Event{Kind: EventCreated, ID: s.id, Kind2: s.kind, Title: s.Title()})
}

func (m *Manager) remove(id string) {
	m.mu.Lock()
	s := m.sessions[id]
	if s == nil {
		m.mu.Unlock()
		return
	}
	delete(m.sessions, id)
	for i, o := range m.order {
		if o == id {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	m.mu.Unlock()
	s.mu.Lock()
	term := s.term
	s.term = nil
	exited := s.state == Exited
	s.mu.Unlock()
	if !exited {
		s.mu.Lock()
		s.state = Exited
		s.mu.Unlock()
	}
	if term != nil {
		_ = term.Close()
	}
	m.broadcast(Event{Kind: EventClosed, ID: s.id, Kind2: s.kind, Title: s.Title()})
}

// Get returns the session with the ID.
func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s := m.sessions[id]
	return s, s != nil
}

// List returns the sessions in creation order.
func (m *Manager) List() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Session, 0, len(m.order))
	for _, id := range m.order {
		if s := m.sessions[id]; s != nil {
			out = append(out, s)
		}
	}
	return out
}

// Count returns how many sessions are still running.
func (m *Manager) Count() int {
	n := 0
	for _, s := range m.List() {
		if s.State() != Exited {
			n++
		}
	}
	return n
}

// CloseAll closes every session, as the app's quit sequence does.
func (m *Manager) CloseAll() {
	for _, s := range m.List() {
		s.Close()
	}
}
