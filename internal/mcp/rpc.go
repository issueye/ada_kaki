// Package mcp is the console's Model Context Protocol server: AI agents
// drive the session engine's terminals over JSON-RPC 2.0, on stdio or
// HTTP. It implements the tools subset of the 2024-11-05 specification.
package mcp

import (
	"encoding/json"
	"fmt"
)

// Protocol versions the server speaks, newest last; initialize picks the
// client's if we know it, else the newest.
var protocolVersions = []string{"2024-11-05", "2025-03-26"}

// JSON-RPC error codes, the standard ones and MCP's server-defined range.
const (
	codeParseError     = -32700
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternal       = -32603
	codeNotInitialized = -32001
)

// request is one JSON-RPC call or notification.
type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// response is one reply, a result or an error.
type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// rpcError is the error member of a response.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *rpcError) Error() string { return fmt.Sprintf("rpc %d: %s", e.Code, e.Message) }

// parseRequest parses one message and validates the envelope.
func parseRequest(data []byte) (*request, *rpcError) {
	var r request
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, &rpcError{Code: codeParseError, Message: "parse error: " + err.Error()}
	}
	if r.JSONRPC != "2.0" || r.Method == "" {
		return nil, &rpcError{Code: codeInvalidParams, Message: "not a JSON-RPC 2.0 request"}
	}
	return &r, nil
}

func marshalResult(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return marshalResult(map[string]any{"error": err.Error()})
	}
	return data
}

func errorResult(code int, message string, data any) json.RawMessage {
	return marshalResult(map[string]any{"isError": true, "error": rpcError{Code: code, Message: message, Data: data}})
}

// ---- the MCP result shapes ----

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type initializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    capabilities `json:"capabilities"`
	ServerInfo      serverInfo   `json:"serverInfo"`
	Instructions    string       `json:"instructions,omitempty"`
}

type capabilities struct {
	Tools *toolsCap `json:"tools,omitempty"`
}

type toolsCap struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

type toolDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema schema `json:"inputSchema"`
}

type schema struct {
	Type       string                `json:"type"`
	Properties map[string]schemaProp `json:"properties,omitempty"`
	Required   []string              `json:"required,omitempty"`
}

type schemaProp struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Default     any      `json:"default,omitempty"`
}

type toolsListResult struct {
	Tools []toolDef `json:"tools"`
}

type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolCallResult struct {
	Content []textContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// textResult builds a successful tool result.
func textResult(format string, args ...any) toolCallResult {
	return toolCallResult{Content: []textContent{{Type: "text", Text: fmt.Sprintf(format, args...)}}}
}

// errorToolResult builds a failed one, as MCP asks: a result, not an RPC
// error, so the model sees what went wrong.
func errorToolResult(format string, args ...any) toolCallResult {
	r := textResult(format, args...)
	r.IsError = true
	return r
}
