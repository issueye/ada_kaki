// Package audit is the console's audit trail: one JSON line per MCP tool
// call, who ran what, when, on which session, and how it went. The AI's
// work stays accountable.
package audit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Entry is one tool call as the audit trail records it.
type Entry struct {
	Time        time.Time `json:"time"`
	Transport   string    `json:"transport"` // "stdio" | "http"
	Client      string    `json:"client,omitempty"`
	Tool        string    `json:"tool"`
	Args        string    `json:"args,omitempty"` // redacted JSON
	Session     string    `json:"session,omitempty"`
	Command     string    `json:"command,omitempty"` // what ran, the highest-value fact
	OK          bool      `json:"ok"`
	Err         string    `json:"error,omitempty"`
	Guard       string    `json:"guard,omitempty"` // "denied:<reason>" when blocked
	OutputBytes int       `json:"output_bytes,omitempty"`
	DurationMS  int64     `json:"duration_ms"`
}

// Log is an append-only JSON-lines file that rotates at a size limit:
// audit.log, audit.log.1, ... The tail of the newest file is what the
// viewer shows.
type Log struct {
	mu      sync.Mutex
	path    string
	max     int64 // bytes per file
	keep    int   // rotated files kept
	f       *os.File
	size    int64
	written int64 // total entries, for the viewer's counter
}

// Open opens (creating if needed) the audit log at path with the default
// rotation: 5 MB a file, 3 old files kept.
func Open(path string) (*Log, error) {
	l := &Log{path: path, max: 5 << 20, keep: 3}
	if err := l.open(); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *Log) open() error {
	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	l.f, l.size = f, st.Size()
	return nil
}

// Record appends one entry, rotating when the file is full.
func (l *Log) Record(e Entry) error {
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	line = append(line, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		if err := l.open(); err != nil {
			return err
		}
	}
	if l.size+int64(len(line)) > l.max {
		if err := l.rotateLocked(); err != nil {
			return err
		}
	}
	n, err := l.f.Write(line)
	l.size += int64(n)
	l.written++
	return err
}

func (l *Log) rotateLocked() error {
	l.f.Close()
	for i := l.keep; i >= 1; i-- {
		old := rotationName(l.path, i)
		next := rotationName(l.path, i+1)
		if i == l.keep {
			os.Remove(old)
		} else if _, err := os.Stat(old); err == nil {
			os.Rename(old, next)
		}
	}
	os.Rename(l.path, rotationName(l.path, 1))
	return l.open()
}

func rotationName(path string, n int) string {
	if n == 0 {
		return path
	}
	return fmt.Sprintf("%s.%d", path, n)
}

// Written tells how many entries were recorded, for the viewer's counter.
func (l *Log) Written() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.written
}

// Tail returns the last n entries, oldest first, across the newest and
// its rotations as far back as needed.
func (l *Log) Tail(n int) ([]Entry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []Entry
	// newest file first, then the rotations
	for gen := 0; gen <= l.keep && len(out) < n; gen++ {
		path := rotationName(l.path, gen)
		f, err := os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return out, err
		}
		lines, err := lastLines(f, n-len(out))
		f.Close()
		if err != nil {
			return out, err
		}
		// this file's lines, oldest first, prepend what we keep
		batch := make([]Entry, 0, len(lines))
		for _, line := range lines {
			var e Entry
			if json.Unmarshal(line, &e) == nil {
				batch = append(batch, e)
			}
		}
		out = append(batch, out...)
	}
	return out, nil
}

// lastLines reads a file's last n lines.
func lastLines(f *os.File, n int) ([][]byte, error) {
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	var lines [][]byte
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			lines = append(lines, []byte(line))
			if len(lines) > n {
				lines = lines[1:]
			}
		}
	}
	return lines, sc.Err()
}

// Close ends the log file.
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}
