// The chat client: one request shape for every provider, streamed or not,
// with readable failures.
package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const maxErrorBody = 4 << 10

// Client talks to one provider with one key.
type Client struct {
	Provider Provider
	Key      string
	HTTP     *http.Client
}

// NewClient builds a client for a provider; a nil http client means the
// default one.
func NewClient(p Provider, key string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{}
	}
	return &Client{Provider: p, Key: key, HTTP: hc}
}

// HTTPError is a non-2xx answer from the provider.
type HTTPError struct {
	Provider string
	Status   int
	Body     string
}

func (e *HTTPError) Error() string {
	reason := ""
	switch e.Status {
	case http.StatusUnauthorized, http.StatusForbidden:
		reason = " rejected the API key"
	case http.StatusNotFound:
		reason = " has no such endpoint or model"
	case http.StatusTooManyRequests:
		reason = " is rate limiting"
	default:
		reason = " returned an error"
	}
	body := strings.TrimSpace(e.Body)
	if body != "" {
		body = ": " + truncate(body, 300)
	}
	return fmt.Sprintf("ai: %s%s (HTTP %d)%s", e.Provider, reason, e.Status, body)
}

// Chat runs one turn. With onDelta non-nil the answer is streamed and the
// callback sees each step; the assembled Response is returned either way.
func (c *Client) Chat(ctx context.Context, req Request, onDelta func(Delta)) (Response, error) {
	if c.Provider.Endpoint() == "" {
		return Response{}, fmt.Errorf("ai: %s: base URL is empty", c.Provider.Name)
	}
	if c.Key == "" {
		return Response{}, fmt.Errorf("%w: %s needs %s", ErrNoAPIKey, c.Provider.Name, c.Provider.KeyEnv)
	}

	stream := onDelta != nil
	payload, err := json.Marshal(c.body(req, stream))
	if err != nil {
		return Response{}, fmt.Errorf("ai: encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Provider.Endpoint(),
		bytes.NewReader(payload))
	if err != nil {
		return Response{}, fmt.Errorf("ai: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.Key)
	if stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	} else {
		httpReq.Header.Set("Accept", "application/json")
	}

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Response{}, ctxErr
		}
		return Response{}, fmt.Errorf("ai: %s: %w", c.Provider.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		return Response{}, &HTTPError{Provider: c.Provider.Name, Status: resp.StatusCode, Body: string(body)}
	}

	if !stream {
		return c.decodeJSON(resp.Body)
	}
	return c.parseStream(ctx, resp.Body, onDelta)
}

// body builds the wire request.
func (c *Client) body(req Request, stream bool) requestBody {
	out := requestBody{
		Model:       c.Provider.Model,
		Messages:    wireMessages(req.Messages),
		Stream:      stream,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}
	if stream {
		out.StreamOptions = &streamOptions{IncludeUsage: true}
	}
	if req.Reasoning != "" && req.Reasoning != "off" {
		out.ReasoningEffort = req.Reasoning
	}
	for _, t := range req.Tools {
		schema := t.Schema
		if len(schema) == 0 {
			schema = []byte(`{"type":"object","properties":{}}`)
		}
		out.Tools = append(out.Tools, wireTool{
			Type: "function",
			Function: wireFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  json.RawMessage(schema),
			},
		})
	}
	return out
}

type requestBody struct {
	Model           string         `json:"model"`
	Messages        []wireMessage  `json:"messages"`
	Stream          bool           `json:"stream,omitempty"`
	StreamOptions   *streamOptions `json:"stream_options,omitempty"`
	Tools           []wireTool     `json:"tools,omitempty"`
	MaxTokens       int            `json:"max_tokens,omitempty"`
	Temperature     *float64       `json:"temperature,omitempty"`
	ReasoningEffort string         `json:"reasoning_effort,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type wireMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type wireToolCall struct {
	ID       string       `json:"id,omitempty"`
	Type     string       `json:"type,omitempty"`
	Function wireFunction `json:"function"`
}

type wireTool struct {
	Type     string       `json:"type"`
	Function wireFunction `json:"function"`
}

type wireFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Arguments   json.RawMessage `json:"arguments,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

func wireMessages(msgs []Message) []wireMessage {
	out := make([]wireMessage, 0, len(msgs))
	for _, m := range msgs {
		wm := wireMessage{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID}
		for _, tc := range m.ToolCalls {
			args := tc.Args
			if args == "" {
				args = "{}"
			}
			wm.ToolCalls = append(wm.ToolCalls, wireToolCall{
				ID: tc.ID, Type: "function",
				Function: wireFunction{Name: tc.Name, Arguments: json.RawMessage(args)},
			})
		}
		out = append(out, wm)
	}
	return out
}

// decodeJSON reads a non-streamed answer.
func (c *Client) decodeJSON(r io.Reader) (Response, error) {
	var raw chatResponse
	if err := json.NewDecoder(io.LimitReader(r, 8<<20)).Decode(&raw); err != nil {
		return Response{}, fmt.Errorf("ai: decode response: %w", err)
	}
	if raw.Error != nil && raw.Error.Message != "" {
		return Response{}, fmt.Errorf("ai: %s: %s", c.Provider.Name, raw.Error.Message)
	}
	if len(raw.Choices) == 0 {
		return Response{}, fmt.Errorf("ai: %s returned no choices", c.Provider.Name)
	}
	out := Response{Finish: raw.Choices[0].FinishReason}
	if raw.Usage != nil {
		out.Usage = *raw.Usage
	}
	out.Content = raw.Choices[0].Message.Content
	for _, tc := range raw.Choices[0].Message.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, ToolCall{
			ID: tc.ID, Name: tc.Function.Name, Args: tc.Function.Arguments,
		})
	}
	return out, nil
}

// parseStream reads server-sent events into one Response, merging tool-call
// fragments by index.
func (c *Client) parseStream(ctx context.Context, r io.Reader, onDelta func(Delta)) (Response, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)

	var out Response
	var content strings.Builder
	calls := map[int]*ToolCall{}
	var order []int

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return out, fmt.Errorf("ai: stream: %w", err)
		}
		if chunk.Error != nil && chunk.Error.Message != "" {
			return out, fmt.Errorf("ai: %s: %s", c.Provider.Name, chunk.Error.Message)
		}
		if chunk.Usage != nil {
			out.Usage = *chunk.Usage
			onDelta(Delta{Usage: chunk.Usage})
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				content.WriteString(choice.Delta.Content)
				out.Content = content.String()
				onDelta(Delta{Content: choice.Delta.Content})
			}
			for _, part := range choice.Delta.ToolCalls {
				tc, ok := calls[part.Index]
				if !ok {
					tc = &ToolCall{}
					calls[part.Index] = tc
					order = append(order, part.Index)
				}
				if part.ID != "" {
					tc.ID = part.ID
				}
				if part.Function.Name != "" {
					tc.Name = part.Function.Name
				}
				tc.Args += part.Function.Arguments
				snapshot := *tc
				onDelta(Delta{ToolCall: &snapshot})
			}
			if choice.FinishReason != "" {
				out.Finish = choice.FinishReason
				onDelta(Delta{Finish: choice.FinishReason})
			}
		}
	}
	if err := sc.Err(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return out, ctxErr
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return out, err
		}
		return out, fmt.Errorf("ai: read stream: %w", err)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return out, ctxErr
	}
	for _, idx := range order {
		tc := calls[idx]
		out.ToolCalls = append(out.ToolCalls, *tc)
	}
	return out, nil
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *Usage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *Usage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
