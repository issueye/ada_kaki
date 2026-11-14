package ui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"adakaki/internal/engine"
	"adakaki/internal/mcp"
	"adakaki/internal/store"
	"adakaki/internal/tools"
)

// waitFor polls until fn is true, for tests.
func waitFor(t *testing.T, d time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(40 * time.Millisecond)
	}
	t.Fatal("timed out")
}

func TestMCPEndpointAuthAndCall(t *testing.T) {
	a := &app{
		mgr:   engine.NewManager(),
		store: &store.Config{},
	}
	defer a.mgr.CloseAll()
	server := mcp.NewServer("adakaki-test", "0", tools.NewBackend(a.mgr, a.store))
	ts := httptest.NewServer(a.mcpMux(server, "sekrit"))
	defer ts.Close()

	post := func(token string, body string) (*http.Response, string) {
		req, _ := http.NewRequest("POST", ts.URL+"/mcp", bytes.NewReader([]byte(body)))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var buf bytes.Buffer
		buf.ReadFrom(resp.Body)
		return resp, buf.String()
	}

	// No token: rejected.
	resp, _ := post("", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no token: status %d; want 401", resp.StatusCode)
	}

	// initialize → tools/list → run_command, over HTTP.
	resp, body := post("sekrit", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`)
	if resp.StatusCode != 200 || !strings.Contains(body, "adakaki-test") {
		t.Errorf("initialize: %d %s", resp.StatusCode, body)
	}
	resp, body = post("sekrit", `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if resp.StatusCode != 200 || !strings.Contains(body, "run_command") {
		t.Errorf("tools/list: %d %s", resp.StatusCode, body)
	}

	reqBody, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 3, "method": "tools/call",
		"params": map[string]any{
			"name": "run_command",
			"arguments": map[string]any{
				"shell": "cmd", "command": "echo http_ok",
				"timeout_ms": 15000, "quiet_ms": 700,
			},
		},
	})
	resp, body = post("sekrit", string(reqBody))
	if resp.StatusCode != 200 || !strings.Contains(body, "http_ok") {
		t.Errorf("run_command: %d %s", resp.StatusCode, body)
	}

	// healthz needs no token and reports sessions.
	hresp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer hresp.Body.Close()
	if hresp.StatusCode != 200 {
		t.Errorf("healthz: %d", hresp.StatusCode)
	}
}

// TestMCPCallMasksSession drives a slow tool call over HTTP and checks
// the session is masked (agent-locked) for the call's whole duration,
// then released.
func TestMCPCallMasksSession(t *testing.T) {
	a := &app{mgr: engine.NewManager(), store: &store.Config{}}
	defer a.mgr.CloseAll()
	backend := tools.NewBackend(a.mgr, a.store)
	backend.SetAgentTracker(func(id, tool string) func() {
		s, ok := a.mgr.Get(id)
		if !ok {
			return func() {}
		}
		s.SetAgentLock(true, tool)
		return func() { s.SetAgentLock(false, "") }
	})
	server := mcp.NewServer("adakaki-test", "0", backend)
	ts := httptest.NewServer(a.mcpMux(server, "sekrit"))
	defer ts.Close()

	post := func(body string) string {
		req, _ := http.NewRequest("POST", ts.URL+"/mcp", bytes.NewReader([]byte(body)))
		req.Header.Set("Authorization", "Bearer sekrit")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var buf bytes.Buffer
		buf.ReadFrom(resp.Body)
		return buf.String()
	}
	call := func(id int, name string, args map[string]any) string {
		reqBody, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0", "id": id, "method": "tools/call",
			"params": map[string]any{"name": name, "arguments": args},
		})
		return post(string(reqBody))
	}

	// A session to operate on (initialize first, as clients must).
	post(`{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`)
	res := post(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_session","arguments":{"shell":"cmd"}}}`)
	var created struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(res), &created); err != nil {
		t.Fatalf("create_session response: %v", err)
	}
	if created.Error != nil {
		t.Fatalf("create_session rpc error %d: %s", created.Error.Code, created.Error.Message)
	}
	if len(created.Result.Content) == 0 {
		t.Fatalf("create_session → %s", res)
	}
	var info struct {
		ID string `json:"id"`
	}
	json.Unmarshal([]byte(created.Result.Content[0].Text), &info)
	if info.ID == "" {
		t.Fatalf("create_session → %s", res)
	}
	s, ok := a.mgr.Get(info.ID)
	if !ok {
		t.Fatal("session not found")
	}

	// A slow command: the lock must hold while the call runs.
	done := make(chan string, 1)
	go func() {
		done <- call(2, "run_command", map[string]any{
			"id": info.ID, "command": "ping -n 2 127.0.0.1 >nul & echo SLOW_DONE",
			"timeout_ms": 20000, "quiet_ms": 500,
		})
	}()

	waitFor(t, 10*time.Second, func() bool {
		on, tool := s.AgentLock()
		return on && tool == "run_command"
	})
	select {
	case res := <-done:
		t.Fatalf("run_command finished before the lock was even seen: %s", res)
	default:
	}

	res = <-done
	if !strings.Contains(res, "SLOW_DONE") {
		t.Errorf("run_command → %s", res)
	}
	waitFor(t, 5*time.Second, func() bool {
		on, _ := s.AgentLock()
		return !on
	})
}
