// Package ai is the built-in AI operator's model side: an
// OpenAI-compatible chat/completions provider, its credentials and token
// accounting. It knows nothing about the console's UI or tools; the agent
// package drives it (docs/AI-功能规划与设计.md §F5).
package ai

import (
	"errors"
	"strings"
)

// Provider is one chat/completions endpoint and the model to ask.
type Provider struct {
	Name    string // display/login name, used in diagnostics
	BaseURL string // e.g. http://127.0.0.1:3050/v1
	Model   string // e.g. deepseek-v4.1-flash-eco
	KeyEnv  string // the environment variable that holds the API key
}

// Endpoint is the provider's chat/completions URL.
func (p Provider) Endpoint() string {
	base := strings.TrimRight(p.BaseURL, "/")
	if base == "" {
		return ""
	}
	return base + "/chat/completions"
}

// ErrNoAPIKey means the credential environment variable is empty.
var ErrNoAPIKey = errors.New("ai: no API key")

// Message is one turn of the conversation, in the shape the provider takes.
type Message struct {
	Role    string `json:"role"` // "system" | "user" | "assistant" | "tool"
	Content string `json:"content"`
	// ToolCalls are the assistant's requested calls; ToolCallID names the
	// call a "tool" message answers.
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall is one function the model asked to run.
type ToolCall struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
	Args string `json:"arguments"`
}

// Tool is a declaration the provider offers the model.
type Tool struct {
	Name        string
	Description string
	// Schema is the tool's JSON Schema for its arguments.
	Schema []byte
}

// Usage is the provider's token accounting for one call.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Total returns the tokens counted, provider value first.
func (u Usage) Total() int {
	if u.TotalTokens > 0 {
		return u.TotalTokens
	}
	return u.PromptTokens + u.CompletionTokens
}

// Request is one chat turn.
type Request struct {
	Messages    []Message
	Tools       []Tool
	MaxTokens   int
	Reasoning   string // "off" | "low" | "medium" | "high", "" to omit
	Stream      bool
	Temperature *float64
}

// Response is the assembled answer.
type Response struct {
	Content   string
	ToolCalls []ToolCall
	Usage     Usage
	Finish    string
}

// Delta is one streamed step handed to the caller's callback.
type Delta struct {
	Content  string
	ToolCall *ToolCall // a new or extended call, already merged
	Usage    *Usage
	Finish   string
}
