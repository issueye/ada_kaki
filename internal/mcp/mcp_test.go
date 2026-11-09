package mcp

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"adakaki/internal/engine"
	"adakaki/internal/store"
)

// client is a test MCP client over pipes, mirroring what an AI client
// does on stdio.
type client struct {
	in     io.WriteCloser
	out    *bufio.Reader
	server *Server
}

func newClient(t *testing.T) (*client, *Server) {
	t.Helper()
	r, w := io.Pipe()
	or, ow := io.Pipe()
	mgr := engine.NewManager()
	t.Cleanup(mgr.CloseAll)
	s := NewServer("adakaki-test", "0.0.1", NewEngineBackend(mgr, &store.Config{}))
	go func() { _ = s.ServeStdio(r, ow) }()
	t.Cleanup(func() { w.Close(); or.Close() })
	return &client{in: w, out: bufio.NewReader(or), server: s}, s
}

// call sends one request and returns the result object.
func (c *client) call(t *testing.T, id int, method string, params map[string]any) map[string]any {
	t.Helper()
	msg := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.in.Write(append(data, '\n')); err != nil {
		t.Fatalf("write %s: %v", method, err)
	}
	return c.readResponse(t, id)
}

// readResponse reads until the response with the id arrives.
func (c *client) readResponse(t *testing.T, id int) map[string]any {
	t.Helper()
	for {
		line, err := c.out.ReadString('\n')
		if err != nil {
			t.Fatalf("read response for %d: %v", id, err)
		}
		var resp struct {
			ID    float64          `json:"id"`
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
			Result map[string]any `json:"result"`
		}
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("bad response %q: %v", line, err)
		}
		if int(resp.ID) != id {
			continue // a notification or another call's reply
		}
		if resp.Error != nil {
			t.Fatalf("%d: rpc error %d: %s", id, resp.Error.Code, resp.Error.Message)
		}
		return resp.Result
	}
}

// resultText pulls the first text content out of a tools/call result.
func resultText(t *testing.T, result map[string]any) string {
	t.Helper()
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("result has no content: %v", result)
	}
	first, _ := content[0].(map[string]any)
	text, _ := first["text"].(string)
	return text
}

func TestInitializeAndList(t *testing.T) {
	c, _ := newClient(t)

	res := c.call(t, 1, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"clientInfo":      map[string]any{"name": "test", "version": "0"},
	})
	if res["protocolVersion"] != "2024-11-05" {
		t.Errorf("protocolVersion = %v", res["protocolVersion"])
	}
	info, _ := res["serverInfo"].(map[string]any)
	if info["name"] != "adakaki-test" {
		t.Errorf("serverInfo = %v", info)
	}

	// The initialized notification draws no reply; ping answers.
	if _, err := c.in.Write([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n")); err != nil {
		t.Fatal(err)
	}
	c.call(t, 2, "ping", nil)

	res = c.call(t, 3, "tools/list", nil)
	tools, _ := res["tools"].([]any)
	if len(tools) != len(toolDefs()) || len(tools) < 10 {
		t.Errorf("tools/list = %d tools; want %d", len(tools), len(toolDefs()))
	}
}

func TestBeforeInitializeRejected(t *testing.T) {
	r, w := io.Pipe()
	or, ow := io.Pipe()
	defer w.Close()
	defer or.Close()
	mgr := engine.NewManager()
	defer mgr.CloseAll()
	s := NewServer("t", "0", NewEngineBackend(mgr, &store.Config{}))
	go func() { _ = s.ServeStdio(r, ow) }()
	c := &client{in: w, out: bufio.NewReader(or)}

	msg := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n")
	c.in.Write(msg)
	line, err := c.out.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(line, `"code":-32001`) {
		t.Errorf("response %q does not refuse an uninitialized client", line)
	}
}

func TestSessionToolLoop(t *testing.T) {
	c, _ := newClient(t)
	c.call(t, 1, "initialize", map[string]any{"protocolVersion": "2024-11-05"})

	// A custom one-shot session echoes and exits.
	text := resultText(t, c.call(t, 2, "tools/call", map[string]any{
		"name": "create_session",
		"arguments": map[string]any{
			"shell": "custom", "command": "cmd.exe /c echo mcp_loop_ok",
		},
	}))
	var created struct {
		ID string `json:"id"`
	}
	json.Unmarshal([]byte(text), &created)
	if created.ID == "" {
		t.Fatalf("create_session → %s", text)
	}

	// The output becomes readable, now or soon.
	deadline := time.Now().Add(10 * time.Second)
	for {
		text = resultText(t, c.call(t, 3, "tools/call", map[string]any{
			"name": "read_session", "arguments": map[string]any{"id": created.ID, "wait_ms": 500},
		}))
		if strings.Contains(text, "mcp_loop_ok") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("read_session never showed the echo; last %q", text)
		}
	}

	// run_command drives an interactive session end to end.
	text = resultText(t, c.call(t, 4, "tools/call", map[string]any{
		"name": "run_command",
		"arguments": map[string]any{
			"shell": "cmd", "command": "echo run_cmd_ok", "timeout_ms": 15000, "quiet_ms": 700,
		},
	}))
	if !strings.Contains(text, "run_cmd_ok") {
		t.Errorf("run_command output missing the echo: %s", text)
	}
	var run struct {
		Session string `json:"session"`
	}
	json.Unmarshal([]byte(text), &run)
	if run.Session == "" {
		t.Errorf("run_command result has no session: %s", text)
	}

	// close_session removes it.
	text = resultText(t, c.call(t, 5, "tools/call", map[string]any{
		"name": "close_session", "arguments": map[string]any{"id": created.ID},
	}))
	if !strings.Contains(text, `"ok":true`) {
		t.Errorf("close_session → %s", text)
	}
	text = resultText(t, c.call(t, 6, "tools/call", map[string]any{
		"name": "list_sessions", "arguments": nil,
	}))
	if strings.Contains(text, created.ID) {
		t.Errorf("closed session %s still listed: %s", created.ID, text)
	}
}

func TestUnknownToolIsAnErrorResult(t *testing.T) {
	c, _ := newClient(t)
	c.call(t, 1, "initialize", map[string]any{"protocolVersion": "2024-11-05"})
	res := c.call(t, 2, "tools/call", map[string]any{
		"name": "nope", "arguments": nil,
	})
	if res["isError"] != true {
		t.Errorf("unknown tool result = %v; want isError", res)
	}
}
