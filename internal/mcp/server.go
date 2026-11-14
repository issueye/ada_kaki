package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"adakaki/internal/audit"
)

// Server answers JSON-RPC 2.0 with the MCP tools subset, over any
// transport that feeds it lines.
type Server struct {
	name      string
	version   string
	backend   Backend
	transport string // "stdio" | "http", for the audit trail
	hooks     Hooks

	mu          sync.Mutex
	initialized bool
	clientName  string // the initializing client's name, for the audit
}

// Call is one tool call as the rails see it, before it runs.
type Call struct {
	Transport string
	Client    string
	Tool      string
	Session   string
	Command   string // what would run, extracted from the arguments
}

// Hooks are the console's rails around tool calls: the guard judges
// before execution (it may block on a human), the audit records every
// call, denied ones included.
type Hooks struct {
	Guard func(c Call) (verdict string, reason string) // "allow" | "deny" | "confirm_ok"
	Audit func(e audit.Entry)
}

// SetTransport names the transport for audit entries.
func (s *Server) SetTransport(t string) {
	s.mu.Lock()
	s.transport = t
	s.mu.Unlock()
}

// SetHooks installs the rails; both are optional.
func (s *Server) SetHooks(h Hooks) {
	s.mu.Lock()
	s.hooks = h
	s.mu.Unlock()
}

// Backend is what the tools drive: the console's session engine, shared
// with the GUI or standalone.
type Backend interface {
	ListShells() (string, error)
	ListSessions() (string, error)
	CreateSession(shell, cwd, command string) (string, error)
	SSHConnectProfile(profile string) (string, error)
	SSHConnect(host string, port int, user, authKind, password, keyPath, passphrase string) (string, error)
	ListSSHProfiles() (string, error)
	WriteSession(id, data string) (string, error)
	ReadSession(id string, waitMS, maxChars int) (string, error)
	ScreenSession(id string) (string, error)
	ResizeSession(id string, cols, rows int) (string, error)
	CloseSession(id string) (string, error)
	RunCommand(id, shell, cwd, command string, timeoutMS, quietMS int) (string, error)
	ReconnectSession(id string) (string, error)
}

// NewServer returns a server the backend serves.
func NewServer(name, version string, backend Backend) *Server {
	return &Server{name: name, version: version, backend: backend}
}

// Handle answers one message, "" for none (a notification).
func (s *Server) Handle(line []byte) []byte {
	req, rerr := parseRequest(line)
	if rerr != nil {
		// A parse error still deserves an answer with a null id.
		return s.reply([]byte("null"), nil, rerr)
	}
	if req.ID == nil {
		return nil // a notification, such as notifications/initialized
	}
	result, err := s.dispatch(req)
	return s.reply(req.ID, result, err)
}

func (s *Server) reply(id json.RawMessage, result any, err *rpcError) []byte {
	resp := response{JSONRPC: "2.0", ID: id}
	if err != nil {
		resp.Error = err
	} else {
		resp.Result = marshalResult(result)
	}
	data, e := json.Marshal(resp)
	if e != nil {
		return []byte(`{"jsonrpc":"2.0","id":null,"error":{"code":-32603,"message":"marshal"}}`)
	}
	return data
}

func (s *Server) dispatch(req *request) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		return s.initialize(req)
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		if !s.ready() {
			return nil, &rpcError{Code: codeNotInitialized, Message: "not initialized: send initialize first"}
		}
		return toolsListResult{Tools: toolDefs()}, nil
	case "tools/call":
		if !s.ready() {
			return nil, &rpcError{Code: codeNotInitialized, Message: "not initialized: send initialize first"}
		}
		return s.toolsCall(req)
	default:
		return nil, &rpcError{Code: codeMethodNotFound, Message: "method " + req.Method + " is not served"}
	}
}

func (s *Server) ready() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.initialized
}

// initialize negotiates the protocol version and answers who we are.
func (s *Server) initialize(req *request) (any, *rpcError) {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
		ClientInfo      struct {
			Name string `json:"name"`
		} `json:"clientInfo"`
	}
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &rpcError{Code: codeInvalidParams, Message: "bad initialize params: " + err.Error()}
		}
	}
	s.mu.Lock()
	s.clientName = p.ClientInfo.Name
	s.mu.Unlock()
	// The client's version when we speak it, else our newest.
	version := protocolVersions[len(protocolVersions)-1]
	for _, v := range protocolVersions {
		if p.ProtocolVersion == v {
			version = v
		}
	}
	s.mu.Lock()
	s.initialized = true
	s.mu.Unlock()
	return initializeResult{
		ProtocolVersion: version,
		Capabilities:    capabilities{Tools: &toolsCap{}},
		ServerInfo:      serverInfo{Name: s.name, Version: s.version},
		Instructions: "AdaKaki console: create terminal sessions (cmd/PowerShell/Git Bash/SSH), " +
			"write commands with a trailing \\r, and read output with read_session or run_command.",
	}, nil
}

// toolsCall runs one tool by name, with the rails around it: the guard
// judges before anything runs, the audit records every call, denied ones
// included.
func (s *Server) toolsCall(req *request) (any, *rpcError) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpcError{Code: codeInvalidParams, Message: "bad tools/call params: " + err.Error()}
	}
	var args map[string]any
	if len(p.Arguments) > 0 {
		if err := json.Unmarshal(p.Arguments, &args); err != nil {
			return nil, &rpcError{Code: codeInvalidParams, Message: "bad arguments: " + err.Error()}
		}
	}
	if args == nil {
		args = map[string]any{}
	}

	start := time.Now()
	s.mu.Lock()
	transport, client, hooks := s.transport, s.clientName, s.hooks
	s.mu.Unlock()
	call := extractCall(p.Name, args)
	call.Transport, call.Client = transport, client
	confirmed := false

	// The guard judges before anything runs; its own failure denies.
	if hooks.Guard != nil {
		verdict, reason := "allow", ""
		func() {
			defer func() {
				if r := recover(); r != nil {
					verdict, reason = "deny", fmt.Sprintf("护栏内部错误: %v", r)
				}
			}()
			verdict, reason = hooks.Guard(call)
		}()
		if verdict == "deny" {
			s.record(hooks, audit.Entry{
				Time: time.Now(), Transport: transport, Client: client,
				Tool: p.Name, Args: redactArgs(args), Session: call.Session,
				Command: call.Command, OK: false,
				Guard: "denied:" + reason,
			})
			return errorToolResult("已被安全护栏拦截：%s", reason), nil
		}
		confirmed = verdict == "confirm_ok"
	}

	result, err := s.runTool(p.Name, args)
	entry := audit.Entry{
		Time: time.Now(), Transport: transport, Client: client,
		Tool: p.Name, Args: redactArgs(args), Session: call.Session,
		Command: call.Command, DurationMS: time.Since(start).Milliseconds(),
	}
	if confirmed {
		entry.Guard = "confirmed"
	}
	if err != nil {
		entry.Err = err.Error()
	} else {
		entry.OK = !result.IsError
		if result.IsError {
			entry.Err = firstText(result)
		}
		entry.OutputBytes = len(firstText(result))
	}
	s.record(hooks, entry)
	if err != nil {
		return errorToolResult("%v", err), nil
	}
	return result, nil
}

// runTool dispatches one tool to the backend.
func (s *Server) runTool(name string, args map[string]any) (toolCallResult, error) {
	b := s.backend
	switch name {
	case "list_shells":
		return textAny(b.ListShells())
	case "list_sessions":
		return textAny(b.ListSessions())
	case "create_session":
		return textAny(b.CreateSession(argString(args, "shell"), argString(args, "cwd"), argString(args, "command")))
	case "ssh_connect":
		if profile := argString(args, "profile"); profile != "" {
			return textAny(b.SSHConnectProfile(profile))
		}
		return textAny(b.SSHConnect(
			argString(args, "host"), argInt(args, "port"), argString(args, "user"),
			argString(args, "auth_kind"), argString(args, "password"),
			argString(args, "key_path"), argString(args, "passphrase")))
	case "list_ssh_profiles":
		return textAny(b.ListSSHProfiles())
	case "write_session":
		return textAny(b.WriteSession(argString(args, "id"), argString(args, "data")))
	case "read_session":
		return textAny(b.ReadSession(argString(args, "id"), argInt(args, "wait_ms"), argInt(args, "max_chars")))
	case "screen_session":
		return textAny(b.ScreenSession(argString(args, "id")))
	case "resize_session":
		return textAny(b.ResizeSession(argString(args, "id"), argInt(args, "cols"), argInt(args, "rows")))
	case "close_session":
		return textAny(b.CloseSession(argString(args, "id")))
	case "reconnect_session":
		return textAny(b.ReconnectSession(argString(args, "id")))
	case "run_command":
		return textAny(b.RunCommand(
			argString(args, "id"), argString(args, "shell"), argString(args, "cwd"), argString(args, "command"),
			argInt(args, "timeout_ms"), argInt(args, "quiet_ms")))
	default:
		return errorToolResult("unknown tool %q", name), nil
	}
}

// extractCall pulls the facts the rails judge: the session and what the
// call would run.
func extractCall(tool string, args map[string]any) Call {
	c := Call{Tool: tool}
	if id, ok := args["id"].(string); ok {
		c.Session = id
	}
	switch tool {
	case "run_command":
		c.Command, _ = args["command"].(string)
	case "write_session":
		if d, ok := args["data"].(string); ok {
			c.Command = submittedLine(d)
		}
	}
	return c
}

// submittedLine returns the line a write would submit — the text before
// its trailing Enter; empty when the write does not submit anything
// (typing inside a program, escape responses).
func submittedLine(data string) string {
	if !strings.HasSuffix(data, "\r") && !strings.HasSuffix(data, "\n") {
		return ""
	}
	trimmed := strings.TrimRight(data, "\r\n")
	if i := strings.LastIndexAny(trimmed, "\r\n"); i >= 0 {
		return trimmed[i+1:]
	}
	return trimmed
}

// redactArgs serializes arguments for the audit, credentials masked.
func redactArgs(args map[string]any) string {
	safe := make(map[string]any, len(args))
	for k, v := range args {
		switch strings.ToLower(k) {
		case "password", "passphrase", "secret":
			if _, ok := v.(string); ok && v != "" {
				safe[k] = "***"
				continue
			}
		}
		safe[k] = v
	}
	data, _ := json.Marshal(safe)
	return string(data)
}

// firstText returns a result's first text content, for the audit.
func firstText(r toolCallResult) string {
	if len(r.Content) > 0 {
		return r.Content[0].Text
	}
	return ""
}

func (s *Server) record(hooks Hooks, e audit.Entry) {
	if hooks.Audit != nil {
		hooks.Audit(e)
	}
}

// textAny wraps a backend JSON string as a tool result.
func textAny(s string, err error) (toolCallResult, error) {
	return textResult("%s", s), err
}

func argString(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

func argInt(args map[string]any, key string) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case string:
		n := 0
		fmt.Sscanf(v, "%d", &n)
		return n
	}
	return 0
}

// ServeStdio runs the server over line-delimited JSON on r, answering on
// w. It returns when r ends. Logs go to stderr, never to w.
func (s *Server) ServeStdio(r io.Reader, w io.Writer) error {
	in := bufio.NewReaderSize(r, 1<<20)
	out := bufio.NewWriter(w)
	for {
		line, err := in.ReadBytes('\n')
		if len(line) > 0 {
			trimmed := trimLine(line)
			if len(trimmed) > 0 {
				if resp := s.Handle(trimmed); resp != nil {
					out.Write(resp)
					out.WriteByte('\n')
					out.Flush()
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func trimLine(line []byte) []byte {
	for len(line) > 0 && (line[len(line)-1] == '\n' || line[len(line)-1] == '\r' || line[len(line)-1] == ' ' || line[len(line)-1] == '\t') {
		line = line[:len(line)-1]
	}
	return line
}
