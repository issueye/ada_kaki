// Package agent is the built-in AI operator's loop: it asks the model, runs
// the tools it asks for through the shared tool layer and the console's
// guardrail, and reports every step as an event the dock renders
// (docs/AI-功能规划与设计.md §F3, §F6, §F7).
package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"adakaki/internal/ai"
	"adakaki/internal/audit"
	"adakaki/internal/tools"
)

// EventKind names what happened in a turn.
type EventKind string

const (
	// EventTurnStarted opens a turn.
	EventTurnStarted EventKind = "turn_started"
	// EventThinking means the model is being asked.
	EventThinking EventKind = "thinking"
	// EventAssistant is the model's prose answer.
	EventAssistant EventKind = "assistant"
	// EventToolStarted precedes a tool call.
	EventToolStarted EventKind = "tool_started"
	// EventToolFinished reports its outcome.
	EventToolFinished EventKind = "tool_finished"
	// EventTurnDone closes a turn, with the reason it stopped.
	EventTurnDone EventKind = "turn_done"
	// EventError reports a turn that could not continue.
	EventError EventKind = "error"
)

// Event is one step the UI observes. Turn events carry the counters.
type Event struct {
	Kind      EventKind
	Tool      string
	Cmd       string
	Out       string
	Err       string
	OK        bool
	MS        int64
	ToolCalls int
	Tokens    int
	Reason    string // turn_done: "done" | "budget" | "tokens" | "canceled"
}

// Chatter is the model side, satisfied by *ai.Client.
type Chatter interface {
	Chat(ctx context.Context, req ai.Request, onDelta func(ai.Delta)) (ai.Response, error)
}

// Runner is the tool side, satisfied by *tools.Backend.
type Runner interface {
	Call(name, args string) (string, error)
}

// Guard judges a tool call before it runs: "allow", "confirm_ok" or "deny",
// with a reason for the refusal.
type Guard interface {
	Check(tool, command string) (verdict string, reason string)
}

// Options configures an agent. Unset budgets and hooks take safe defaults.
type Options struct {
	Chatter           Chatter
	Runner            Runner
	Guard             Guard
	MaxToolCalls      int // per turn; 0 means 40
	SessionTokenLimit int // per session; 0 means 200k
	Session           string
	Transport         string // audit transport; "" means "inapp"
	ClientName        string // audit client; "" means "adakaki-ai"
	Reasoning         string
	MaxTokens         int
	Emit              func(Event)
	Audit             func(audit.Entry)
	Now               func() time.Time
	SystemPrompt      string
}

// Agent keeps the conversation history across turns and the token total.
type Agent struct {
	opts    Options
	history []ai.Message
	tokens  int
}

const defaultSystemPrompt = `You are AdaKaki's built-in terminal operator. You run in a desktop console with several
shell sessions (local cmd/PowerShell/Git Bash, SSH hosts). Use the tools to inspect sessions
and run commands; prefer run_command for one-shot work, and read_session or screen_session
to see the result. The console's guardrail may ask the user before dangerous commands run;
if a call is blocked, say so plainly and suggest a safer alternative. Answer in the user's
language, keep replies short, and never claim success you did not observe in a tool result.`

// New returns an agent; options missing budgets get the documented defaults.
func New(o Options) *Agent {
	if o.MaxToolCalls <= 0 {
		o.MaxToolCalls = 40
	}
	if o.SessionTokenLimit <= 0 {
		o.SessionTokenLimit = 200_000
	}
	if o.Transport == "" {
		o.Transport = "inapp"
	}
	if o.ClientName == "" {
		o.ClientName = "adakaki-ai"
	}
	if o.Emit == nil {
		o.Emit = func(Event) {}
	}
	if o.Audit == nil {
		o.Audit = func(audit.Entry) {}
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.SystemPrompt == "" {
		o.SystemPrompt = defaultSystemPrompt
	}
	return &Agent{opts: o}
}

// Tokens is the session's token total as far as the provider reported it.
func (a *Agent) Tokens() int { return a.tokens }

// History returns a copy of the conversation, for diagnostics.
func (a *Agent) History() []ai.Message {
	out := make([]ai.Message, len(a.history))
	copy(out, a.history)
	return out
}

// SetSession tells the agent which session the next turns operate, for the
// audit trail and the system prompt.
func (a *Agent) SetSession(id string) { a.opts.Session = id }

// Turn asks the model with a new prompt and runs whatever tools it requests
// until it answers, the budget runs out, or the turn is cancelled.
func (a *Agent) Turn(ctx context.Context, prompt string) error {
	a.opts.Emit(Event{Kind: EventTurnStarted})
	a.history = append(a.history, ai.Message{Role: "user", Content: prompt})

	usedTools := 0
	for {
		if err := ctx.Err(); err != nil {
			a.opts.Emit(Event{Kind: EventTurnDone, ToolCalls: usedTools,
				Tokens: a.tokens, Reason: "canceled"})
			return err
		}

		a.opts.Emit(Event{Kind: EventThinking})
		resp, err := a.opts.Chatter.Chat(ctx, a.request(), nil)
		if err != nil {
			a.opts.Emit(Event{Kind: EventError, Err: err.Error()})
			return err
		}
		a.tokens += resp.Usage.Total()

		a.history = append(a.history, assistantMessage(resp))
		if resp.Content != "" {
			a.opts.Emit(Event{Kind: EventAssistant, Out: resp.Content})
		}
		if len(resp.ToolCalls) == 0 {
			a.opts.Emit(Event{Kind: EventTurnDone, ToolCalls: usedTools,
				Tokens: a.tokens, Reason: "done"})
			return nil
		}
		if a.tokens >= a.opts.SessionTokenLimit {
			a.opts.Emit(Event{Kind: EventTurnDone, ToolCalls: usedTools,
				Tokens: a.tokens, Reason: "tokens"})
			return nil
		}

		for _, call := range resp.ToolCalls {
			if usedTools >= a.opts.MaxToolCalls {
				a.history = append(a.history, toolMessage(call,
					fmt.Sprintf("not run: the turn's tool budget (%d) is spent; ask the user to continue.",
						a.opts.MaxToolCalls)))
				a.opts.Emit(Event{Kind: EventTurnDone, ToolCalls: usedTools,
					Tokens: a.tokens, Reason: "budget"})
				return nil
			}
			usedTools++
			a.runTool(ctx, call)
		}
	}
}

// runTool guards, runs and records one call, then feeds its result back.
func (a *Agent) runTool(ctx context.Context, call ai.ToolCall) {
	cmd := tools.CommandOf(call.Name, call.Args)
	a.opts.Emit(Event{Kind: EventToolStarted, Tool: call.Name, Cmd: cmd})

	verdict, reason := "allow", ""
	if a.opts.Guard != nil {
		verdict, reason = a.opts.Guard.Check(call.Name, cmd)
	}

	start := a.opts.Now()
	out := ""
	ok := true
	errText := ""
	var callErr error
	switch {
	case ctx.Err() != nil:
		callErr = ctx.Err()
		ok = false
	case verdict == "deny":
		ok = false
		errText = "blocked by the console guardrail: " + reason
		out = errText
	default:
		out, callErr = a.opts.Runner.Call(call.Name, call.Args)
		if callErr != nil {
			ok = false
			errText = callErr.Error()
			out = "error: " + errText
		}
	}
	ms := a.opts.Now().Sub(start).Milliseconds()

	if ok {
		a.history = append(a.history, toolMessage(call, out))
	} else {
		a.history = append(a.history, toolMessage(call, out))
	}

	a.opts.Emit(Event{Kind: EventToolFinished, Tool: call.Name, Cmd: cmd,
		Out: out, OK: ok, Err: errText, MS: ms})

	entry := audit.Entry{
		Time:        start,
		Transport:   a.opts.Transport,
		Client:      a.opts.ClientName,
		Tool:        call.Name,
		Args:        truncate(call.Args, 1024),
		Session:     a.opts.Session,
		Command:     cmd,
		OK:          ok,
		Err:         errText,
		OutputBytes: len(out),
		DurationMS:  ms,
	}
	if verdict == "deny" {
		entry.Guard = "denied:" + reason
	}
	a.opts.Audit(entry)
}

// request builds the provider call from the history.
func (a *Agent) request() ai.Request {
	msgs := make([]ai.Message, 0, len(a.history)+1)
	msgs = append(msgs, ai.Message{Role: "system", Content: a.opts.SystemPrompt})
	msgs = append(msgs, a.history...)
	req := ai.Request{Messages: msgs, Reasoning: a.opts.Reasoning,
		MaxTokens: a.opts.MaxTokens}
	for _, spec := range tools.Specs() {
		req.Tools = append(req.Tools, ai.Tool{
			Name: spec.Name, Description: spec.Description, Schema: spec.Schema,
		})
	}
	return req
}

func assistantMessage(resp ai.Response) ai.Message {
	return ai.Message{Role: "assistant", Content: resp.Content, ToolCalls: resp.ToolCalls}
}

func toolMessage(call ai.ToolCall, out string) ai.Message {
	return ai.Message{Role: "tool", Content: out, ToolCallID: call.ID}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// Summarize compresses a tool result for the dock's one-line card.
func Summarize(out string, limit int) string {
	s := strings.TrimSpace(out)
	s = strings.Join(strings.Fields(s), " ")
	if limit <= 0 {
		limit = 80
	}
	if len([]rune(s)) <= limit {
		return s
	}
	return string([]rune(s)[:limit]) + "…"
}
