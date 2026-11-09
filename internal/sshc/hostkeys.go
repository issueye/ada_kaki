package sshc

import (
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
)

// HostKeyStore is the console's known_hosts file, OpenSSH format:
//
//	host keytype base64          (port 22)
//	[host]:port keytype base64   (other ports)
//
// It reads only what this app writes (no hashed entries).
type HostKeyStore struct {
	mu   sync.Mutex
	path string
	keys map[string]storedKey // "[host]:port" → key
}

type storedKey struct {
	Type        string
	Fingerprint string
	Base64      string
}

// LoadHostKeys reads the known_hosts file, an empty store when there is
// none yet.
func LoadHostKeys(path string) (*HostKeyStore, error) {
	s := &HostKeyStore{path: path, keys: map[string]storedKey{}}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		s.keys[parts[0]] = storedKey{Type: parts[1], Base64: parts[2]}
	}
	return s, nil
}

func hostKeyID(host string, port int) string {
	if port == 22 || port == 0 {
		return host
	}
	return fmt.Sprintf("[%s]:%d", host, port)
}

// Get returns the stored key's fingerprint for a host, known or not.
func (s *HostKeyStore) Get(host string, port int) (fingerprint string, known bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.keys[hostKeyID(host, port)]
	if !ok {
		return "", false
	}
	// The fingerprint is derived from the key itself; keep it current.
	if pub, err := base64Key(k.Type, k.Base64); err == nil {
		return ssh.FingerprintSHA256(pub), true
	}
	return k.Fingerprint, true
}

// Add records a host's key and saves.
func (s *HostKeyStore) Add(host string, port int, key ssh.PublicKey) error {
	s.mu.Lock()
	s.keys[hostKeyID(host, port)] = storedKey{
		Type:   key.Type(),
		Base64: base64KeyString(key),
	}
	s.mu.Unlock()
	return s.Save()
}

// Remove forgets a host and saves.
func (s *HostKeyStore) Remove(host string, port int) error {
	s.mu.Lock()
	delete(s.keys, hostKeyID(host, port))
	s.mu.Unlock()
	return s.Save()
}

// List returns every host with its fingerprint, sorted, for the UI.
func (s *HostKeyStore) List() []KnownHost {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]KnownHost, 0, len(s.keys))
	for id, k := range s.keys {
		fp := k.Fingerprint
		if pub, err := base64Key(k.Type, k.Base64); err == nil {
			fp = ssh.FingerprintSHA256(pub)
		}
		host, port := parseHostKeyID(id)
		out = append(out, KnownHost{Host: host, Port: port, Type: k.Type, Fingerprint: fp})
	}
	// a stable order for the UI
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Host < out[j-1].Host; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// KnownHost is one entry of the store, for the UI.
type KnownHost struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Type        string `json:"type"`
	Fingerprint string `json:"fingerprint"`
}

// Save writes the file back, creating the directory.
func (s *HostKeyStore) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var b strings.Builder
	for id, k := range s.keys {
		fmt.Fprintf(&b, "%s %s %s\n", id, k.Type, k.Base64)
	}
	if err := os.MkdirAll(dirOf(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// HostKeyPolicy decides what an SSH server's key may do, as TOFU does:
// a known key must match, an unknown one is asked about or stored, and a
// CHANGED key is always refused — that is what a man in the middle
// looks like.
type HostKeyPolicy struct {
	// Store records trusted keys; nil means verify nothing (off).
	Store *HostKeyStore
	// AcceptNew stores keys of unknown hosts without asking (headless).
	AcceptNew bool
	// Prompt asks the user about an unknown host's key; nil with
	// !AcceptNew refuses unknown hosts. It runs on the dial goroutine
	// and may block on the user.
	Prompt func(host string, port int, fingerprint string) bool
	// Insecure skips every check (the setting "off").
	Insecure bool
}

// CallbackFor builds the SSH client's host key callback, bound to the
// host and port the dial is about, so the store can key by both.
func (p *HostKeyPolicy) CallbackFor(host string, port int) ssh.HostKeyCallback {
	if p == nil || p.Insecure || p.Store == nil {
		return ssh.InsecureIgnoreHostKey()
	}
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		return p.check(host, port, key)
	}
}

// check runs the TOFU rules for one presented key.
func (p *HostKeyPolicy) check(host string, port int, key ssh.PublicKey) error {
	fp := ssh.FingerprintSHA256(key)
	if stored, known := p.Store.Get(host, port); known {
		if stored == fp {
			return nil
		}
		return &HostKeyChangedError{Host: host, Stored: stored, Got: fp}
	}
	if p.Prompt != nil {
		if !p.Prompt(host, port, fp) {
			return fmt.Errorf("ssh: 已取消连接 %s", host)
		}
		return p.Store.Add(host, port, key)
	}
	if p.AcceptNew {
		return p.Store.Add(host, port, key)
	}
	return fmt.Errorf("ssh: 主机 %s 未知（指纹 %s），未连接；在设置中改为自动信任或提示确认后重试", host, fp)
}

// HostKeyChangedError says a known host answered with another key.
type HostKeyChangedError struct {
	Host   string
	Stored string
	Got    string
}

func (e *HostKeyChangedError) Error() string {
	return fmt.Sprintf(
		"ssh: 主机 %s 的密钥已变化，连接被拒绝（可能是中间人）！\n  已存指纹: %s\n  本次指纹: %s\n若确为对方换钥，请在设置的已信任主机中删除 %s 后重试",
		e.Host, e.Stored, e.Got, e.Host)
}

// ---- small helpers ----

func base64KeyString(key ssh.PublicKey) string {
	return base64.StdEncoding.EncodeToString(key.Marshal())
}

// base64Key parses a stored key back from its base64 wire form.
func base64Key(keyType, b64 string) (ssh.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	return ssh.ParsePublicKey(raw)
}

func dirOf(path string) string {
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		return path[:i]
	}
	if i := strings.LastIndexByte(path, '\\'); i >= 0 {
		return path[:i]
	}
	return "."
}

// parseHostKeyID splits "[host]:port" or plain "host" back apart.
func parseHostKeyID(id string) (host string, port int) {
	if strings.HasPrefix(id, "[") {
		if i := strings.Index(id, "]:"); i > 0 {
			host = id[1:i]
			fmt.Sscanf(id[i+2:], "%d", &port)
			return host, port
		}
	}
	return id, 22
}
