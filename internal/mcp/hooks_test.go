package mcp

import (
	"strings"
	"testing"

	"adakaki/internal/audit"
)

// TestHooksGuardAndAudit runs a guarded, audited server end to end:
// dangerous commands are denied with a clear result, every call lands in
// the audit trail, credentials are redacted, and the client's name from
// initialize is attached.
func TestHooksGuardAndAudit(t *testing.T) {
	c, _ := newClient(t)
	var entries []audit.Entry
	srv := c.server
	srv.SetTransport("stdio")
	srv.SetHooks(Hooks{
		Guard: func(call Call) (string, string) {
			if strings.Contains(call.Command, "rm -rf") {
				return "deny", "递归/强制删除（rm -rf）"
			}
			return "allow", ""
		},
		Audit: func(e audit.Entry) { entries = append(entries, e) },
	})

	c.call(t, 1, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"clientInfo":      map[string]any{"name": "guard-test", "version": "0"},
	})

	// A denied call: an error result that names the rail, audited.
	res := c.call(t, 2, "tools/call", map[string]any{
		"name": "run_command",
		"arguments": map[string]any{
			"shell": "cmd", "command": "echo ok && rm -rf build",
			"timeout_ms": 5000, "quiet_ms": 300,
		},
	})
	text := resultText(t, res)
	if !strings.Contains(text, "安全护栏") || !strings.Contains(text, "rm -rf") {
		t.Errorf("denied result = %s", text)
	}
	if res["isError"] != true {
		t.Error("a denied call must be isError")
	}
	if len(entries) != 1 || !strings.HasPrefix(entries[0].Guard, "denied") {
		t.Fatalf("audit after deny = %+v", entries)
	}
	if entries[0].Client != "guard-test" || entries[0].Transport != "stdio" {
		t.Errorf("audit client/transport = %q/%q", entries[0].Client, entries[0].Transport)
	}
	if entries[0].Command != "echo ok && rm -rf build" {
		t.Errorf("audit command = %q", entries[0].Command)
	}

	// An allowed call is audited too, with ok set.
	res = c.call(t, 3, "tools/call", map[string]any{
		"name": "run_command",
		"arguments": map[string]any{
			"shell": "cmd", "command": "echo safe_command",
			"timeout_ms": 15000, "quiet_ms": 700,
		},
	})
	if !strings.Contains(resultText(t, res), "safe_command") {
		t.Errorf("allowed run_command → %s", resultText(t, res))
	}
	if len(entries) != 2 || entries[1].Guard != "" || !entries[1].OK {
		t.Fatalf("audit after allow = %+v", entries)
	}
	if entries[1].OutputBytes == 0 {
		t.Error("output size not recorded")
	}

	// Credentials never reach the audit trail.
	res = c.call(t, 4, "tools/call", map[string]any{
		"name": "ssh_connect",
		"arguments": map[string]any{
			"host": "nowhere.invalid", "user": "u",
			"auth_kind": "password", "password": "super-secret",
		},
	})
	if len(entries) != 3 {
		t.Fatalf("%d entries after ssh_connect", len(entries))
	}
	if strings.Contains(entries[2].Args, "super-secret") {
		t.Errorf("the password leaked into the audit: %s", entries[2].Args)
	}
	if !strings.Contains(entries[2].Args, "***") {
		t.Errorf("the redaction mark is missing: %s", entries[2].Args)
	}
}

// TestWriteSessionGuardExtraction checks that a submitted write's last
// line is what the guard judges, and typing without Enter is not judged
// at all.
func TestWriteSessionGuardExtraction(t *testing.T) {
	calls := map[string]string{}
	c, _ := newClient(t)
	c.server.SetHooks(Hooks{
		Guard: func(call Call) (string, string) {
			calls[call.Tool+"|"+call.Command] = call.Command
			return "allow", ""
		},
	})
	c.call(t, 1, "initialize", map[string]any{"protocolVersion": "2024-11-05"})

	// vim keys without Enter: guarded on nothing.
	c.call(t, 2, "tools/call", map[string]any{
		"name": "write_session", "arguments": map[string]any{"id": "x", "data": "j"},
	})
	// A submitted line: the last one is what counts.
	c.call(t, 3, "tools/call", map[string]any{
		"name": "write_session", "arguments": map[string]any{"id": "x", "data": "ls\r\necho danger\r"},
	})
	if _, ok := calls["write_session|j"]; ok {
		t.Error("a write without Enter must not carry a command")
	}
	if calls["write_session|echo danger"] == "" {
		t.Errorf("the submitted line was not extracted: %v", calls)
	}
}

// TestSubmittedLine checks what counts as a submitted command line.
func TestSubmittedLine(t *testing.T) {
	cases := []struct{ data, want string }{
		{"echo hi\r", "echo hi"},
		{"echo hi\r\n", "echo hi"},
		{"ls\r\npwd\r", "pwd"},
		{"npm install --save\r", "npm install --save"},
		{"j", ""},      // typing inside a pager: no submit, no guard
		{"\x1b[A", ""}, // an arrow key
	}
	for _, c := range cases {
		if got := submittedLine(c.data); got != c.want {
			t.Errorf("submittedLine(%q) = %q; want %q", c.data, got, c.want)
		}
	}
}
