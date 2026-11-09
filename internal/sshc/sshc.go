// Package sshc is the console's SSH client: interactive shells wired to
// the terminal plugin's Conn interface, and one-shot execs for MCP tools.
//
// Host keys are not verified in v1 (InsecureIgnoreHostKey), as the design
// document notes; known_hosts support comes later.
package sshc

import (
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// Auth is one way to prove who the user is.
type Auth struct {
	Kind       string // "password" | "key"
	Password   string
	KeyPath    string
	Passphrase string
}

// Client wraps one SSH connection.
type Client struct {
	*ssh.Client
	host      string
	user      string
	agentConn io.ReadWriteCloser // the agent pipe, closed with the client
}

// Dial connects and authenticates. A nil policy verifies nothing; pass
// a HostKeyPolicy for TOFU.
func Dial(host string, port int, user string, auth Auth, policy *HostKeyPolicy) (*Client, error) {
	if port == 0 {
		port = 22
	}
	cfg := &ssh.ClientConfig{
		User:    user,
		Timeout: 15 * time.Second,
	}
	methods, agentConn, err := authMethods(auth)
	if err != nil {
		return nil, err
	}
	cfg.Auth = methods
	cfg.HostKeyCallback = policy.CallbackFor(host, port)

	addr := net.JoinHostPort(host, fmt.Sprint(port))
	raw, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, fmt.Errorf("ssh: %s: %w", addr, err)
	}
	return &Client{Client: raw, host: host, user: user, agentConn: agentConn}, nil
}

// authMethods builds the auth methods for a dial; the agent kind also
// returns the connection to close when the client ends.
func authMethods(a Auth) ([]ssh.AuthMethod, io.ReadWriteCloser, error) {
	switch a.Kind {
	case "password", "":
		if a.Password == "" {
			return nil, nil, fmt.Errorf("ssh: no password given")
		}
		return []ssh.AuthMethod{ssh.Password(a.Password)}, nil, nil
	case "key":
		signer, err := readKey(a.KeyPath, a.Passphrase)
		if err != nil {
			return nil, nil, err
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil, nil
	case "agent":
		conn, err := openAgent()
		if err != nil {
			return nil, nil, err
		}
		method := ssh.PublicKeysCallback(agent.NewClient(conn).Signers)
		return []ssh.AuthMethod{method}, conn, nil
	default:
		return nil, nil, fmt.Errorf("ssh: unknown auth kind %q", a.Kind)
	}
}

// Close ends the connection and the agent pipe with it.
func (c *Client) Close() error {
	if c.agentConn != nil {
		_ = c.agentConn.Close()
	}
	return c.Client.Close()
}

func readKey(path, passphrase string) (ssh.Signer, error) {
	if path == "" {
		return nil, fmt.Errorf("ssh: no key path given")
	}
	pem, err := readFileLimited(path, 1<<20)
	if err != nil {
		return nil, fmt.Errorf("ssh: %w", err)
	}
	if passphrase != "" {
		s, err := ssh.ParsePrivateKeyWithPassphrase(pem, []byte(passphrase))
		if err != nil {
			return nil, fmt.Errorf("ssh: key %s: %w", path, err)
		}
		return s, nil
	}
	s, err := ssh.ParsePrivateKey(pem)
	if err != nil {
		return nil, fmt.Errorf("ssh: key %s: %w", path, err)
	}
	return s, nil
}

func readFileLimited(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if st.Size() > limit {
		return nil, fmt.Errorf("%s is too large to be a key", path)
	}
	data := make([]byte, st.Size())
	if _, err := f.Read(data); err != nil && err != io.EOF {
		return nil, err
	}
	return data, nil
}

// Host returns the address the client dialed, for titles.
func (c *Client) Host() string { return c.host }

// User returns the account the client logged in as.
func (c *Client) User() string { return c.user }

// shellConn is one interactive shell over SSH, shaped as the terminal
// plugin's Conn: the plugin reads the remote's output from it and writes
// what is typed to it, and Resize follows the view.
type shellConn struct {
	client  *Client
	session *ssh.Session
	stdin   io.WriteCloser

	outR *io.PipeReader // merged stdout+stderr, what the plugin reads

	mu       sync.Mutex
	closed   bool
	closeErr error
	wg       sync.WaitGroup
}

// OpenShell starts an interactive shell with a PTY of the given size.
func (c *Client) OpenShell(cols, rows int) (*shellConn, error) {
	session, err := c.NewSession()
	if err != nil {
		return nil, fmt.Errorf("ssh: session: %w", err)
	}
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 115200,
		ssh.TTY_OP_OSPEED: 115200,
	}
	if err := session.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		session.Close()
		return nil, fmt.Errorf("ssh: pty: %w", err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		return nil, err
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		session.Close()
		return nil, err
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		return nil, err
	}
	// Both remote streams feed one pipe; it closes when both end, so the
	// plugin's Read sees the connection's end.
	r, w := io.Pipe()
	k := &shellConn{client: c, session: session, stdin: stdin, outR: r}
	k.wg.Add(2)
	go func() { defer k.wg.Done(); io.Copy(w, stdout); w.Close() }()
	go func() { defer k.wg.Done(); io.Copy(w, stderr) }()
	if err := session.Shell(); err != nil {
		k.Close()
		return nil, fmt.Errorf("ssh: shell: %w", err)
	}
	return k, nil
}

// Read reads the shell's output, terminal escape sequences included.
func (k *shellConn) Read(p []byte) (int, error) { return k.outR.Read(p) }

// Write sends what is typed to the shell.
func (k *shellConn) Write(b []byte) (int, error) { return k.stdin.Write(b) }

// Resize tells the remote the window's size in cells. The plugin looks
// the method up by type and calls it as the view resizes.
func (k *shellConn) Resize(cols, rows int) error {
	return k.session.WindowChange(rows, cols) // height first, as x/crypto takes it
}

// Close closes the shell and the connection. Read returns once the
// remote's output is drained.
func (k *shellConn) Close() error {
	k.mu.Lock()
	if k.closed {
		err := k.closeErr
		k.mu.Unlock()
		return err
	}
	k.closed = true
	k.mu.Unlock()

	k.session.Close()
	k.outR.Close()
	k.wg.Wait()
	err := k.client.Close()
	k.mu.Lock()
	k.closeErr = err
	k.mu.Unlock()
	return err
}

// Exec runs one command and returns its combined output and exit status.
func (c *Client) Exec(cmd string, timeout time.Duration) (string, int, error) {
	session, err := c.NewSession()
	if err != nil {
		return "", -1, fmt.Errorf("ssh: session: %w", err)
	}
	defer session.Close()

	var buf limitedBuffer
	buf.limit = 512 << 10
	session.Stdout = &buf
	session.Stderr = &buf

	done := make(chan error, 1)
	go func() { done <- session.Run(cmd) }()

	var runErr error
	select {
	case runErr = <-done:
	case <-time.After(timeout):
		session.Close()
		return buf.String(), -1, fmt.Errorf("ssh: command timed out after %s", timeout)
	}
	exit := 0
	if runErr != nil {
		if e, ok := runErr.(*ssh.ExitError); ok {
			exit = e.ExitStatus()
		} else {
			return buf.String(), -1, runErr
		}
	}
	return buf.String(), exit, nil
}

// limitedBuffer keeps Exec's output within a sane size.
type limitedBuffer struct {
	limit int
	b     []byte
}

func (w *limitedBuffer) Write(p []byte) (int, error) {
	if room := w.limit - len(w.b); room > 0 {
		if len(p) > room {
			w.b = append(w.b, p[:room]...)
		} else {
			w.b = append(w.b, p...)
		}
	}
	return len(p), nil // the output counts as taken even when truncated
}

func (w *limitedBuffer) String() string { return string(w.b) }
