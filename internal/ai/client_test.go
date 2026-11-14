package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testProvider(url string) Provider {
	return Provider{Name: "test-provider", BaseURL: url, Model: "m-1", KeyEnv: "TEST_KEY"}
}

func sseServer(handler func(w http.ResponseWriter, flusher http.Flusher)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		handler(w, w.(http.Flusher))
	}))
}

// writeEvent sends one SSE data event built from a JSON-able value.
func writeEvent(t *testing.T, w http.ResponseWriter, f http.Flusher, payload any) {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	fmt.Fprintf(w, "data: %s\n\n", data)
	f.Flush()
}

// A streamed answer accumulates content, usage and finish reason.
func TestChatStreamText(t *testing.T) {
	srv := sseServer(func(w http.ResponseWriter, f http.Flusher) {
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hel\"}}]}\n\n")
		f.Flush()
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"lo\"},\"finish_reason\":\"stop\"}]}\n\n")
		f.Flush()
		fmt.Fprintf(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5,\"total_tokens\":15}}\n\n")
		f.Flush()
		fmt.Fprintln(w, "data: [DONE]")
	})
	defer srv.Close()

	var steps []Delta
	c := NewClient(testProvider(srv.URL), "k", nil)
	resp, err := c.Chat(context.Background(), Request{
		Messages: []Message{{Role: "user", Content: "hi"}},
	}, func(d Delta) { steps = append(steps, d) })
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "hello" {
		t.Fatalf("content = %q, want hello", resp.Content)
	}
	if resp.Finish != "stop" {
		t.Fatalf("finish = %q, want stop", resp.Finish)
	}
	if resp.Usage.Total() != 15 {
		t.Fatalf("usage total = %d, want 15", resp.Usage.Total())
	}
	if len(steps) < 3 {
		t.Fatalf("expected at least 3 deltas, got %d", len(steps))
	}
}

// Tool-call fragments arriving in pieces merge into one call by index.
func TestChatStreamToolCalls(t *testing.T) {
	srv := sseServer(func(w http.ResponseWriter, f http.Flusher) {
		writeEvent(t, w, f, map[string]any{
			"choices": []any{map[string]any{
				"delta": map[string]any{
					"tool_calls": []any{map[string]any{
						"index": 0, "id": "call_1",
						"function": map[string]any{"name": "run_command", "arguments": `{"cmd":"go `},
					}},
				},
			}},
		})
		writeEvent(t, w, f, map[string]any{
			"choices": []any{map[string]any{
				"delta": map[string]any{
					"tool_calls": []any{map[string]any{
						"index":    0,
						"function": map[string]any{"arguments": `test"}`},
					}},
				},
				"finish_reason": "tool_calls",
			}},
		})
		fmt.Fprintln(w, "data: [DONE]")
	})
	defer srv.Close()

	c := NewClient(testProvider(srv.URL), "k", nil)
	resp, err := c.Chat(context.Background(), Request{}, func(Delta) {})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.ID != "call_1" || tc.Name != "run_command" {
		t.Fatalf("call identity = %q/%q", tc.ID, tc.Name)
	}
	if tc.Args != `{"cmd":"go test"}` {
		t.Fatalf("merged arguments = %q", tc.Args)
	}
}

// A rejected key becomes a readable HTTPError naming the provider.
func TestChatAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"invalid key"}}`)
	}))
	defer srv.Close()

	c := NewClient(testProvider(srv.URL), "k", nil)
	_, err := c.Chat(context.Background(), Request{}, nil)
	var he *HTTPError
	if !errors.As(err, &he) {
		t.Fatalf("expected *HTTPError, got %T: %v", err, err)
	}
	if he.Status != http.StatusUnauthorized {
		t.Fatalf("status = %d", he.Status)
	}
	msg := err.Error()
	if !strings.Contains(msg, "test-provider") || !strings.Contains(msg, "API key") {
		t.Fatalf("error should name the provider and the key: %v", msg)
	}
}

// No key is refused before any request goes out.
func TestChatMissingKey(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
	}))
	defer srv.Close()

	c := NewClient(testProvider(srv.URL), "", nil)
	_, err := c.Chat(context.Background(), Request{}, nil)
	if err == nil || !errors.Is(err, ErrNoAPIKey) {
		t.Fatalf("want ErrNoAPIKey, got %v", err)
	}
	if hits != 0 {
		t.Fatalf("no request should reach the server, got %d", hits)
	}
}

// Non-streamed JSON parses into the same Response shape.
func TestChatJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("Accept = %q, want application/json", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"ok","tool_calls":[{"id":"c2","function":{"name":"list_sessions","arguments":"{}"}}]},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1}}`)
	}))
	defer srv.Close()

	c := NewClient(testProvider(srv.URL), "k", nil)
	resp, err := c.Chat(context.Background(), Request{Messages: []Message{{Role: "user", Content: "x"}}}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "ok" || len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "list_sessions" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.Usage.Total() != 4 {
		t.Fatalf("usage total = %d, want 4", resp.Usage.Total())
	}
}

// A cancelled context stops the stream and returns the context error.
func TestChatCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	c := NewClient(testProvider(srv.URL), "k", nil)
	_, err := c.Chat(ctx, Request{}, func(Delta) {})
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

// A malformed stream is an error, not a silent truncation.
func TestChatMalformedStream(t *testing.T) {
	srv := sseServer(func(w http.ResponseWriter, f http.Flusher) {
		fmt.Fprintf(w, "data: {not json}\n\n")
		f.Flush()
	})
	defer srv.Close()

	c := NewClient(testProvider(srv.URL), "k", nil)
	_, err := c.Chat(context.Background(), Request{}, func(Delta) {})
	if err == nil || !strings.Contains(err.Error(), "stream") {
		t.Fatalf("want a stream error, got %v", err)
	}
}

func TestChatEmptyBaseURL(t *testing.T) {
	c := NewClient(Provider{Name: "p", Model: "m", KeyEnv: "K"}, "k", nil)
	if _, err := c.Chat(context.Background(), Request{}, nil); err == nil {
		t.Fatal("an empty base URL must be an error")
	}
}

func TestProviderEndpoint(t *testing.T) {
	cases := map[string]string{
		"http://127.0.0.1:3050/v1":          "http://127.0.0.1:3050/v1/chat/completions",
		"http://127.0.0.1:3050/v1/":         "http://127.0.0.1:3050/v1/chat/completions",
		"https://api.example.com/openai/v1": "https://api.example.com/openai/v1/chat/completions",
		"":                                  "",
	}
	for base, want := range cases {
		if got := (Provider{BaseURL: base}).Endpoint(); got != want {
			t.Fatalf("Endpoint(%q) = %q, want %q", base, got, want)
		}
	}
}

func TestEstimateTokens(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"", 0},
		{"abcd", 1},
		{"abcdefgh", 2},
		{"测试中文", 2},          // 4 CJK / 2.5 → ceil(1.6)
		{"最后一行", 2},          // 4 CJK
		{"go test ./...", 4}, // 13 latin chars → ceil(13/4)
		{"中文 and English 混排", 6},
	}
	for _, tc := range cases {
		if got := EstimateTokens(tc.text); got != tc.want {
			t.Fatalf("EstimateTokens(%q) = %d, want %d", tc.text, got, tc.want)
		}
	}
}

func TestKeyFromEnv(t *testing.T) {
	if _, err := KeyFromEnv(""); !errors.Is(err, ErrNoAPIKey) {
		t.Fatalf("empty name: want ErrNoAPIKey, got %v", err)
	}
	t.Setenv("AI_TEST_KEY", "secret")
	key, err := KeyFromEnv("AI_TEST_KEY")
	if err != nil || key != "secret" {
		t.Fatalf("KeyFromEnv = %q, %v", key, err)
	}
	if _, err := KeyFromEnv("AI_TEST_MISSING"); !errors.Is(err, ErrNoAPIKey) {
		t.Fatalf("missing variable: want ErrNoAPIKey, got %v", err)
	}
}
