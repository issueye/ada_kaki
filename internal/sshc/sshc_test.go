package sshc

import (
	"bytes"
	"crypto/ed25519"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// mockServer is a local SSH server that accepts a password, grants a
// shell that echoes a banner and repeats what is typed, and runs execs.
type mockServer struct {
	ln net.Listener
}

const mockBanner = "WELCOME_MOCK\r\n"

// mockHostKey is the mock server's FIXED key, so host key policy tests
// can pre-trust it: a deterministic Ed25519 key from a constant seed.
var mockHostKey = ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x5e}, 32))

func newMockServer(t *testing.T, password string) *mockServer {
	t.Helper()
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(meta ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if string(pass) == password && meta.User() == "tester" {
				return &ssh.Permissions{}, nil
			}
			return nil, fmt.Errorf("auth rejected")
		},
		// The mock accepts the one public key the agent test offers.
		PublicKeyCallback: func(meta ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if meta.User() == "agentuser" {
				return &ssh.Permissions{}, nil
			}
			return nil, fmt.Errorf("auth rejected")
		},
	}
	signer, err := ssh.NewSignerFromKey(mockHostKey)
	if err != nil {
		t.Fatal(err)
	}
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	m := &mockServer{ln: ln}
	go m.serve(cfg)
	t.Cleanup(func() { ln.Close() })
	return m
}

func (m *mockServer) addr() string { return m.ln.Addr().String() }

func (m *mockServer) serve(cfg *ssh.ServerConfig) {
	for {
		conn, err := m.ln.Accept()
		if err != nil {
			return
		}
		go m.handle(conn, cfg)
	}
}

func (m *mockServer) handle(conn net.Conn, cfg *ssh.ServerConfig) {
	sconn, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	defer sconn.Close()
	go ssh.DiscardRequests(reqs)
	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			newChan.Reject(ssh.UnknownChannelType, "only sessions")
			continue
		}
		ch, inReq, err := newChan.Accept()
		if err != nil {
			return
		}
		go m.session(ch, inReq)
	}
}

func (m *mockServer) session(ch ssh.Channel, inReq <-chan *ssh.Request) {
	for req := range inReq {
		switch req.Type {
		case "pty-req", "env":
			if req.WantReply {
				req.Reply(true, nil)
			}
		case "shell":
			if req.WantReply {
				req.Reply(true, nil)
			}
			// The banner, then the echo of what is typed.
			fmt.Fprint(ch, mockBanner)
			go func() {
				buf := make([]byte, 512)
				for {
					n, err := ch.Read(buf)
					if n > 0 {
						ch.Write(buf[:n])
					}
					if err != nil {
						sendExit(ch, 0)
						return
					}
				}
			}()
		case "exec":
			var payload struct{ Command string }
			ssh.Unmarshal(req.Payload, &payload)
			if req.WantReply {
				req.Reply(true, nil)
			}
			fmt.Fprintf(ch, "EXEC:%s\n", payload.Command)
			if !strings.Contains(payload.Command, "slow") {
				sendExit(ch, 0)
			}
			// A "slow" command sends no status: the client times out.
		default:
			if req.WantReply {
				req.Reply(false, nil)
			}
		}
	}
}

func sendExit(ch ssh.Channel, code uint32) {
	status := ssh.Marshal(struct{ Status uint32 }{code})
	ch.SendRequest("exit-status", false, status)
	ch.Close() // the client's Run returns once the channel closes
}

// readFor reads what arrives within d: io.Pipe has no deadline.
func readFor(k io.Reader, d time.Duration) ([]byte, error) {
	type res struct {
		n   []byte
		err error
	}
	ch := make(chan res, 1)
	go func() {
		buf := make([]byte, 4096)
		n, err := k.Read(buf)
		ch <- res{buf[:n], err}
	}()
	select {
	case r := <-ch:
		return r.n, r.err
	case <-time.After(d):
		return nil, fmt.Errorf("read timed out after %s", d)
	}
}

func dialMock(t *testing.T, m *mockServer, password string) *Client {
	t.Helper()
	host, port, _ := splitAddr(m.addr())
	c, err := Dial(host, port, "tester", Auth{Kind: "password", Password: password}, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	return c
}

func splitAddr(addr string) (host string, port int, err error) {
	h, p, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, err
	}
	n := 0
	fmt.Sscanf(p, "%d", &n)
	return h, n, nil
}

// ---- tests ----

func TestDialPassword(t *testing.T) {
	m := newMockServer(t, "hunter2")
	c := dialMock(t, m, "hunter2")
	defer c.Close()
	if _, err := Dial(m.addr(), 0, "tester", Auth{Kind: "password", Password: "wrong"}, nil); err == nil {
		t.Error("Dial with a wrong password must fail")
	}
}

func TestOpenShell(t *testing.T) {
	m := newMockServer(t, "pw")
	c := dialMock(t, m, "pw")
	defer c.Close()

	k, err := c.OpenShell(80, 24)
	if err != nil {
		t.Fatalf("OpenShell: %v", err)
	}
	defer k.Close()

	// The banner arrives over the merged stream.
	out, err := readFor(k, 5*time.Second)
	if err != nil && len(out) == 0 {
		t.Fatalf("Read: %v", err)
	}
	if !strings.Contains(string(out), "WELCOME_MOCK") {
		t.Errorf("first output %q does not contain the banner", out)
	}

	// What is typed comes back echoed.
	if _, err := k.Write([]byte("ping\r\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	var echo []byte
	for i := 0; i < 5; i++ {
		out, err = readFor(k, 3*time.Second)
		echo = append(echo, out...)
		if err != nil {
			break
		}
		if strings.Contains(string(echo), "ping") {
			break
		}
	}
	if !strings.Contains(string(echo), "ping") {
		t.Errorf("echo %q does not contain what was typed", echo)
	}

	// Resize reaches the remote as a WindowChange request (the mock
	// answers with no payload; it must not error).
	if err := k.Resize(100, 30); err != nil {
		t.Errorf("Resize: %v", err)
	}
}

func TestExec(t *testing.T) {
	m := newMockServer(t, "pw")
	c := dialMock(t, m, "pw")
	defer c.Close()

	out, code, err := c.Exec("echo hi", 5*time.Second)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if code != 0 {
		t.Errorf("exit = %d; want 0", code)
	}
	if !strings.Contains(out, "EXEC:echo hi") {
		t.Errorf("Exec output %q missing the echo", out)
	}
}

func TestExecTimeout(t *testing.T) {
	m := newMockServer(t, "pw")
	c := dialMock(t, m, "pw")
	defer c.Close()

	// The mock's "slow" exec never sends a status: it times out.
	_, _, err := c.Exec("slow", 300*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("Exec timeout error = %v; want a timeout", err)
	}
}

// ---- host key policy ----

func newPolicyStore(t *testing.T) *HostKeyStore {
	t.Helper()
	s, err := LoadHostKeys(t.TempDir() + "/known_hosts")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPolicyAcceptNewStoresAndRefusesChanged(t *testing.T) {
	m := newMockServer(t, "pw")
	host, port, _ := splitAddr(m.addr())
	store := newPolicyStore(t)
	policy := &HostKeyPolicy{Store: store, AcceptNew: true}

	// First dial: the key is stored, the dial goes through.
	c, err := Dial(host, port, "tester", Auth{Kind: "password", Password: "pw"}, policy)
	if err != nil {
		t.Fatalf("first dial: %v", err)
	}
	c.Close()
	if fp, known := store.Get(host, port); !known || fp == "" {
		t.Fatal("the key was not stored on first use")
	}

	// Same key again: fine.
	c, err = Dial(host, port, "tester", Auth{Kind: "password", Password: "pw"}, policy)
	if err != nil {
		t.Fatalf("second dial: %v", err)
	}
	c.Close()

	// A CHANGED key is refused — the man-in-the-middle case.
	signer, _ := ssh.NewSignerFromKey(ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x11}, 32)))
	stored, _ := store.Get(host, port)
	store.keys[hostKeyID(host, port)] = storedKey{Type: "ssh-ed25519", Base64: base64KeyString(signer.PublicKey())}
	_ = stored
	if _, err := Dial(host, port, "tester", Auth{Kind: "password", Password: "pw"}, policy); err == nil {
		t.Fatal("dial with a changed key must be refused")
	} else if !strings.Contains(err.Error(), "已变化") {
		t.Errorf("changed-key error = %v; want the 指纹已变化 message", err)
	}
}

func TestPolicyPromptHonored(t *testing.T) {
	m := newMockServer(t, "pw")
	host, port, _ := splitAddr(m.addr())
	store := newPolicyStore(t)

	// The user refuses: no connection, nothing stored.
	said := 0
	policy := &HostKeyPolicy{Store: store, Prompt: func(h string, p int, fp string) bool {
		said++
		return false
	}}
	if _, err := Dial(host, port, "tester", Auth{Kind: "password", Password: "pw"}, policy); err == nil {
		t.Fatal("dial after refusal must fail")
	}
	if said != 1 {
		t.Errorf("prompt called %d times; want 1", said)
	}
	if _, known := store.Get(host, port); known {
		t.Error("a refused key must not be stored")
	}

	// The user trusts: stored, and never asked again.
	policy.Prompt = func(h string, p int, fp string) bool { return true }
	c, err := Dial(host, port, "tester", Auth{Kind: "password", Password: "pw"}, policy)
	if err != nil {
		t.Fatalf("dial after trust: %v", err)
	}
	c.Close()
	asked := 0
	policy.Prompt = func(h string, p int, fp string) bool { asked++; return false }
	c, err = Dial(host, port, "tester", Auth{Kind: "password", Password: "pw"}, policy)
	if err != nil {
		t.Fatalf("known host must dial without asking: %v", err)
	}
	c.Close()
	if asked != 0 {
		t.Errorf("a known host prompted %d times", asked)
	}
}

func TestPolicyInsecureAndNil(t *testing.T) {
	m := newMockServer(t, "pw")
	host, port, _ := splitAddr(m.addr())
	// nil policy: verify nothing (the old behavior).
	c, err := Dial(host, port, "tester", Auth{Kind: "password", Password: "pw"}, nil)
	if err != nil {
		t.Fatalf("nil policy dial: %v", err)
	}
	c.Close()
	// Insecure policy: same.
	policy := &HostKeyPolicy{Store: newPolicyStore(t), Insecure: true}
	c, err = Dial(host, port, "tester", Auth{Kind: "password", Password: "pw"}, policy)
	if err != nil {
		t.Fatalf("insecure dial: %v", err)
	}
	c.Close()
	if _, known := policy.Store.Get(host, port); known {
		t.Error("insecure policy must not store keys")
	}
}

// ---- agent auth ----

func TestAgentAuth(t *testing.T) {
	m := newMockServer(t, "pw")
	host, port, _ := splitAddr(m.addr())

	// A little agent on TCP, holding one key the mock server accepts.
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0xa7}, 32))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	ring := agent.NewKeyring()
	if err := ring.Add(agent.AddedKey{PrivateKey: key, Comment: "test"}); err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go agent.ServeAgent(ring, conn)
		}
	}()

	t.Setenv("ADAKAKI_AGENT_ADDR", ln.Addr().String())
	c, err := Dial(host, port, "agentuser", Auth{Kind: "agent"}, nil)
	if err != nil {
		t.Fatalf("agent dial: %v", err)
	}
	c.Close() // must also close the agent connection without hanging

	// No agent at all: a clear error, not a panic.
	t.Setenv("ADAKAKI_AGENT_ADDR", "127.0.0.1:1") // nothing listens here
	if _, err := Dial(host, port, "agentuser", Auth{Kind: "agent"}, nil); err == nil {
		t.Error("agent dial without an agent must fail")
	}
}

// ---- known hosts store ----

func TestHostKeyStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/known_hosts"
	s, err := LoadHostKeys(path)
	if err != nil {
		t.Fatal(err)
	}
	signer, _ := ssh.NewSignerFromKey(mockHostKey)
	if err := s.Add("example.com", 22, signer.PublicKey()); err != nil {
		t.Fatal(err)
	}
	if err := s.Add("example.com", 2222, signer.PublicKey()); err != nil {
		t.Fatal(err)
	}
	// Re-loaded from disk, both entries survive with their fingerprints.
	s2, err := LoadHostKeys(path)
	if err != nil {
		t.Fatal(err)
	}
	fp, known := s2.Get("example.com", 22)
	if !known || fp == "" {
		t.Fatal("port 22 entry lost")
	}
	fp2, known := s2.Get("example.com", 2222)
	if !known || fp2 != fp {
		t.Errorf("port 2222 entry = %q, %v; want the same fingerprint", fp2, known)
	}
	// The file is OpenSSH-compatible in shape: three fields per line.
	data, _ := os.ReadFile(path)
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if n := len(strings.Fields(line)); n < 3 {
			t.Errorf("known_hosts line %q has %d fields", line, n)
		}
	}
	// Remove forgets.
	if err := s2.Remove("example.com", 2222); err != nil {
		t.Fatal(err)
	}
	if _, known := s2.Get("example.com", 2222); known {
		t.Error("entry survived Remove")
	}
	if _, known := s2.Get("example.com", 22); !known {
		t.Error("Remove took the wrong entry")
	}
}
