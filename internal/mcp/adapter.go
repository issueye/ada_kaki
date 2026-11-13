package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"adakaki/internal/engine"
	"adakaki/internal/shells"
	"adakaki/internal/sshc"
	"adakaki/internal/store"
)

// AgentTracker is told when a tool call starts and stops operating a
// session, so a GUI can mask the session while it lasts. Begin returns
// the release to run when the call ends.
type AgentTracker func(sessionID, tool string) (release func())

// EngineBackend serves the MCP tools over the console's session engine.
// The GUI and the stdio mode both use it.
type EngineBackend struct {
	mgr     *engine.Manager
	store   *store.Config
	tracker AgentTracker        // nil on stdio, where there is no GUI to mask
	policy  *sshc.HostKeyPolicy // nil: the headless TOFU default

	mu      sync.Mutex
	cursors map[string]string // session id → the text the last read returned
}

// SetAgentTracker installs the tracker the GUI uses to mask sessions
// under operation.
func (b *EngineBackend) SetAgentTracker(t AgentTracker) {
	b.mu.Lock()
	b.tracker = t
	b.mu.Unlock()
}

// begin marks a session as being operated for the duration of the call.
func (b *EngineBackend) begin(id, tool string) func() {
	b.mu.Lock()
	t := b.tracker
	b.mu.Unlock()
	if t == nil || id == "" {
		return func() {}
	}
	if release := t(id, tool); release != nil {
		return release
	}
	return func() {}
}

// NewEngineBackend returns a backend for a manager. The manager's own
// events clean the read cursors of closed sessions up.
func NewEngineBackend(mgr *engine.Manager, cfg *store.Config) *EngineBackend {
	b := &EngineBackend{mgr: mgr, store: cfg, cursors: map[string]string{}}
	mgr.Subscribe(func(ev engine.Event) {
		if ev.Kind == engine.EventClosed || ev.Kind == engine.EventExited {
			b.mu.Lock()
			if ev.Kind == engine.EventClosed {
				delete(b.cursors, ev.ID)
			}
			b.mu.Unlock()
		}
	})
	return b
}

func toJSON(v any) (string, error) {
	data, err := json.Marshal(v)
	return string(data), err
}

// ---- sessions ----

type sessionInfo struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Title    string `json:"title"`
	State    string `json:"state"`
	ExitCode int    `json:"exit_code,omitempty"`
	ExitErr  string `json:"exit_error,omitempty"`
	Profile  string `json:"profile,omitempty"`
	Shell    string `json:"shell,omitempty"`
	HasView  bool   `json:"has_view"` // shown in a GUI window (always false on stdio)
}

func (b *EngineBackend) sessionInfos() []sessionInfo {
	out := []sessionInfo{}
	for _, s := range b.mgr.List() {
		out = append(out, sessionInfo{
			ID:       s.ID(),
			Kind:     string(s.Kind()),
			Title:    s.Title(),
			State:    string(s.State()),
			ExitCode: s.ExitCode(),
			ExitErr:  s.ExitErr(),
			Profile:  s.Profile(),
			Shell:    string(s.Shell()),
		})
	}
	return out
}

func (b *EngineBackend) ListShells() (string, error) {
	out := []map[string]any{}
	for _, sh := range shells.Detect() {
		out = append(out, map[string]any{"id": string(sh.ID), "name": sh.Name, "found": sh.Found, "path": sh.Path})
	}
	return toJSON(out)
}

func (b *EngineBackend) ListSessions() (string, error) {
	return toJSON(b.sessionInfos())
}

func (b *EngineBackend) CreateSession(shell, cwd, command string) (string, error) {
	id := shells.ID(shell)
	var s *engine.Session
	var err error
	if id == shells.Custom {
		if command == "" {
			return "", fmt.Errorf("create_session: shell \"custom\" needs command")
		}
		s, err = b.mgr.CreateLocal(id, cwd, command)
	} else {
		s, err = b.mgr.CreateLocal(id, cwd, "")
	}
	if err != nil {
		return "", err
	}
	return toJSON(map[string]any{"id": s.ID(), "title": s.Title()})
}

func (b *EngineBackend) WriteSession(id, data string) (string, error) {
	s, ok := b.mgr.Get(id)
	if !ok {
		return "", fmt.Errorf("no session %s", id)
	}
	defer b.begin(id, "write_session")()
	s.Send(data)
	return toJSON(map[string]any{"ok": true})
}

func (b *EngineBackend) ReadSession(id string, waitMS, maxChars int) (string, error) {
	s, ok := b.mgr.Get(id)
	if !ok {
		return "", fmt.Errorf("no session %s", id)
	}
	defer b.begin(id, "read_session")()
	deadline := time.Now().Add(time.Duration(waitMS) * time.Millisecond)
	var delta string
	for {
		text := s.Text()
		b.mu.Lock()
		last := b.cursors[id]
		b.mu.Unlock()
		if strings.HasPrefix(text, last) {
			delta = text[len(last):]
		} else {
			// The screen reset (scrollback trimmed or a full-screen
			// program repainted): everything is new again.
			delta = "\n[screen reset; full text follows]\n" + text
		}
		if delta != "" || waitMS <= 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	b.mu.Lock()
	b.cursors[id] = s.Text()
	b.mu.Unlock()
	if maxChars > 0 && len(delta) > maxChars {
		delta = delta[:maxChars] + "\n…[truncated]"
	}
	return delta, nil
}

func (b *EngineBackend) ScreenSession(id string) (string, error) {
	s, ok := b.mgr.Get(id)
	if !ok {
		return "", fmt.Errorf("no session %s", id)
	}
	defer b.begin(id, "screen_session")()
	text := s.Text()
	_, rows := s.Size()
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if rows < 1 {
		rows = 24
	}
	if len(lines) > rows {
		lines = lines[len(lines)-rows:]
	}
	return strings.Join(lines, "\n"), nil
}

func (b *EngineBackend) ResizeSession(id string, cols, rows int) (string, error) {
	s, ok := b.mgr.Get(id)
	if !ok {
		return "", fmt.Errorf("no session %s", id)
	}
	if cols < 1 || rows < 1 || cols > 1000 || rows > 1000 {
		return "", fmt.Errorf("resize_session: size %dx%d out of range", cols, rows)
	}
	defer b.begin(id, "resize_session")()
	s.Resize(cols, rows)
	return toJSON(map[string]any{"ok": true})
}

func (b *EngineBackend) CloseSession(id string) (string, error) {
	s, ok := b.mgr.Get(id)
	if !ok {
		return "", fmt.Errorf("no session %s", id)
	}
	defer b.begin(id, "close_session")()
	s.Close()
	return toJSON(map[string]any{"ok": true})
}

func (b *EngineBackend) RunCommand(id, shell, cwd, command string, timeoutMS, quietMS int) (string, error) {
	if command == "" {
		return "", fmt.Errorf("run_command: command is required")
	}
	if timeoutMS <= 0 {
		timeoutMS = 10000
	}
	if quietMS <= 0 {
		quietMS = 800
	}
	var s *engine.Session
	if id == "" {
		if shell == "" {
			for _, sh := range shells.Detect() {
				if sh.Found {
					shell = string(sh.ID)
					break
				}
			}
		}
		res, err := b.CreateSession(shell, cwd, "")
		if err != nil {
			return "", err
		}
		var created struct {
			ID string `json:"id"`
		}
		json.Unmarshal([]byte(res), &created)
		id = created.ID
		time.Sleep(600 * time.Millisecond) // let the prompt come up
	} else if _, ok := b.mgr.Get(id); !ok {
		return "", fmt.Errorf("no session %s", id)
	}
	s, _ = b.mgr.Get(id)

	// Collect output from after the command, until quiet or timeout. The
	// whole run counts as the agent operating the session.
	defer b.begin(id, "run_command")()
	b.mu.Lock()
	b.cursors[id] = s.Text()
	b.mu.Unlock()
	s.Send(command + "\r")

	start := time.Now()
	deadline := start.Add(time.Duration(timeoutMS) * time.Millisecond)
	quiet := time.Duration(quietMS) * time.Millisecond
	grace := 3 * quiet // a command that prints nothing still gets a fair wait
	var out strings.Builder
	lastChange := time.Now()
	firstOutput := false
	timedOut := false
	for {
		time.Sleep(60 * time.Millisecond)
		text := s.Text()
		b.mu.Lock()
		last := b.cursors[id]
		b.mu.Unlock()
		var delta string
		if strings.HasPrefix(text, last) {
			delta = text[len(last):]
		} else {
			delta = text
			last = ""
		}
		if delta != "" {
			out.WriteString(delta)
			b.mu.Lock()
			b.cursors[id] = text
			b.mu.Unlock()
			lastChange = time.Now()
			firstOutput = true
		}
		// Quiet means output has stopped since the first line; a command
		// that prints nothing at all waits out the grace instead.
		quieted := firstOutput && time.Since(lastChange) >= quiet
		silent := !firstOutput && time.Since(start) >= grace
		if quieted || silent || time.Now().After(deadline) {
			timedOut = time.Now().After(deadline) && !quieted && !silent
			break
		}
	}
	// The prompt the command ended at is part of what came back; note the
	// exit state, which the AI can also ask list_sessions for.
	result := map[string]any{
		"session":   id,
		"output":    out.String(),
		"state":     string(s.State()),
		"timed_out": timedOut,
	}
	return toJSON(result)
}

// ---- ssh ----

func (b *EngineBackend) SSHConnectProfile(profile string) (string, error) {
	p, ok := b.store.Profile(profile)
	if !ok {
		return "", fmt.Errorf("no SSH profile %s", profile)
	}
	pass, _ := store.Unprotect(p.Secret)
	return b.SSHConnect(p.Host, p.Port, p.User, p.AuthKind, pass, p.KeyPath, pass)
}

func (b *EngineBackend) SSHConnect(host string, port int, user, authKind, password, keyPath, passphrase string) (string, error) {
	if user == "" || host == "" {
		return "", fmt.Errorf("ssh_connect: host and user are required")
	}
	if authKind == "" {
		authKind = "password"
	}
	auth := sshc.Auth{Kind: authKind, Password: password, KeyPath: keyPath, Passphrase: passphrase}
	policy := b.hostKeyPolicy()
	source := func(s *engine.Session) (engine.Conn, string, error) {
		client, err := sshc.Dial(host, port, user, auth, policy)
		if err != nil {
			return nil, "", err
		}
		conn, err := client.OpenShell(80, 24)
		if err != nil {
			_ = client.Close()
			return nil, "", err
		}
		return conn, user + "@" + host, nil
	}
	s, err := b.mgr.CreateSSH("", source)
	if err != nil {
		return "", err
	}
	return toJSON(map[string]any{"id": s.ID(), "title": s.Title()})
}

// ReconnectSession restarts an exited session: same ID, same pane. An
// SSH session dials again; a local one starts its shell or command again.
func (b *EngineBackend) ReconnectSession(id string) (string, error) {
	s, ok := b.mgr.Get(id)
	if !ok {
		return "", fmt.Errorf("no session %s", id)
	}
	defer b.begin(id, "reconnect_session")()
	if err := s.Reconnect(); err != nil {
		return "", err
	}
	return toJSON(map[string]any{"ok": true, "id": s.ID(), "title": s.Title()})
}

// SetHostKeyPolicy installs the SSH host key policy the GUI uses, so
// MCP-made connections follow the same TOFU rules and share the store.
func (b *EngineBackend) SetHostKeyPolicy(p *sshc.HostKeyPolicy) {
	b.mu.Lock()
	b.policy = p
	b.mu.Unlock()
}

// hostKeyPolicy returns the installed policy, or the headless default:
// accept new keys into the store, refuse changed ones.
func (b *EngineBackend) hostKeyPolicy() *sshc.HostKeyPolicy {
	b.mu.Lock()
	p := b.policy
	b.mu.Unlock()
	if p != nil {
		return p
	}
	store, _ := sshc.LoadHostKeys(mustKnownHostsPath())
	return &sshc.HostKeyPolicy{Store: store, AcceptNew: true}
}

func mustKnownHostsPath() string {
	p, err := store.KnownHostsPath()
	if err != nil {
		return "known_hosts"
	}
	return p
}

func (b *EngineBackend) ListSSHProfiles() (string, error) {
	out := []map[string]any{}
	for _, p := range b.store.Profiles {
		out = append(out, map[string]any{
			"name": p.Name, "host": p.Host, "port": p.Port, "user": p.User,
			"auth_kind": p.AuthKind, "key_path": p.KeyPath,
		})
	}
	return toJSON(out)
}
