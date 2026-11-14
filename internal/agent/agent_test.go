package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"adakaki/internal/ai"
	"adakaki/internal/audit"
)

// fakeChatter answers from a queue of scripted responses.
type fakeChatter struct {
	responses []ai.Response
	errs      []error
	requests  []ai.Request
}

func (f *fakeChatter) Chat(_ context.Context, req ai.Request, _ func(ai.Delta)) (ai.Response, error) {
	f.requests = append(f.requests, req)
	if len(f.errs) > 0 && f.errs[0] != nil {
		err := f.errs[0]
		f.errs = f.errs[1:]
		return ai.Response{}, err
	}
	if len(f.responses) == 0 {
		return ai.Response{Content: "no more scripted answers"}, nil
	}
	resp := f.responses[0]
	f.responses = f.responses[1:]
	return resp, nil
}

// fakeRunner records tool calls and returns scripted output or errors.
type fakeRunner struct {
	calls []string
	args  []string
	out   string
	err   error
}

func (f *fakeRunner) Call(name, args string) (string, error) {
	f.calls = append(f.calls, name)
	f.args = append(f.args, args)
	return f.out, f.err
}

type fakeGuard struct {
	verdict string
	reason  string
	calls   []string
}

func (g *fakeGuard) Check(tool, command string) (string, string) {
	g.calls = append(g.calls, tool+":"+command)
	if g.verdict == "" {
		return "allow", ""
	}
	return g.verdict, g.reason
}

type recorder struct {
	events  []Event
	entries []audit.Entry
}

func newRecorder() *recorder { return &recorder{} }

func (r *recorder) emit(e Event)        { r.events = append(r.events, e) }
func (r *recorder) audit(e audit.Entry) { r.entries = append(r.entries, e) }
func (r *recorder) kinds() []EventKind {
	out := make([]EventKind, 0, len(r.events))
	for _, e := range r.events {
		out = append(out, e.Kind)
	}
	return out
}

func newTestAgent(ch *fakeChatter, run *fakeRunner, gd *fakeGuard, rec *recorder, opts ...func(*Options)) *Agent {
	o := Options{
		Chatter: ch, Runner: run, Guard: gd,
		Emit: rec.emit, Audit: rec.audit,
		Session: "local_1",
		Now:     func() time.Time { return time.Unix(0, 0) },
	}
	for _, fn := range opts {
		fn(&o)
	}
	return New(o)
}

func TestTurnPlainAnswer(t *testing.T) {
	ch := &fakeChatter{responses: []ai.Response{{Content: "好了", Usage: ai.Usage{TotalTokens: 10}}}}
	run := &fakeRunner{}
	gd := &fakeGuard{}
	rec := newRecorder()
	a := newTestAgent(ch, run, gd, rec)

	if err := a.Turn(context.Background(), "看看状态"); err != nil {
		t.Fatalf("Turn: %v", err)
	}
	want := []EventKind{EventTurnStarted, EventThinking, EventAssistant, EventTurnDone}
	if got := rec.kinds(); len(got) != len(want) {
		t.Fatalf("events = %v, want %v", got, want)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("event %d = %q, want %q", i, got[i], want[i])
			}
		}
	}
	if len(run.calls) != 0 {
		t.Fatalf("no tool call expected, got %v", run.calls)
	}
	if a.Tokens() != 10 {
		t.Fatalf("tokens = %d, want 10", a.Tokens())
	}
	if last := rec.events[len(rec.events)-1]; last.Reason != "done" {
		t.Fatalf("turn done reason = %q, want done", last.Reason)
	}
	if len(rec.entries) != 0 {
		t.Fatal("a text-only turn should not write audit entries")
	}
}

func TestTurnRunsToolAndAudits(t *testing.T) {
	call := ai.ToolCall{ID: "call_1", Name: "run_command", Args: `{"command":"go test ./..."}`}
	ch := &fakeChatter{responses: []ai.Response{
		{ToolCalls: []ai.ToolCall{call}, Usage: ai.Usage{TotalTokens: 50}},
		{Content: "测试通过", Usage: ai.Usage{TotalTokens: 20}},
	}}
	run := &fakeRunner{out: `{"session":"local_1","output":"ok"}`}
	gd := &fakeGuard{}
	rec := newRecorder()
	a := newTestAgent(ch, run, gd, rec, func(o *Options) { o.Reasoning = "high" })

	if err := a.Turn(context.Background(), "跑测试"); err != nil {
		t.Fatalf("Turn: %v", err)
	}
	if len(run.calls) != 1 || run.calls[0] != "run_command" {
		t.Fatalf("runner calls = %v", run.calls)
	}
	if !strings.Contains(run.args[0], "go test") {
		t.Fatalf("runner args = %q", run.args[0])
	}
	if got := rec.kinds(); len(got) < 5 {
		t.Fatalf("events = %v", got)
	}
	if len(rec.entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(rec.entries))
	}
	e := rec.entries[0]
	if e.Transport != "inapp" || e.Client != "adakaki-ai" || e.Session != "local_1" {
		t.Fatalf("audit identity = %q/%q/%q", e.Transport, e.Client, e.Session)
	}
	if e.Tool != "run_command" || e.Command != "go test ./..." || !e.OK {
		t.Fatalf("audit entry = %+v", e)
	}
	if a.Tokens() != 70 {
		t.Fatalf("tokens = %d, want 70", a.Tokens())
	}
	// user + assistant(call) + tool + assistant(answer)
	if len(a.History()) != 4 {
		t.Fatalf("history = %d messages, want 4", len(a.History()))
	}
	if len(ch.requests) != 2 {
		t.Fatalf("chat calls = %d, want 2", len(ch.requests))
	}
	if got := len(ch.requests[0].Tools); got != 12 {
		t.Fatalf("request offers %d tools, want 12", got)
	}
	if ch.requests[1].Messages[len(ch.requests[1].Messages)-1].Role != "tool" {
		t.Fatal("the second request should carry the tool result")
	}
}

func TestGuardDenyBlocksRun(t *testing.T) {
	call := ai.ToolCall{ID: "c1", Name: "run_command", Args: `{"command":"rm -rf node_modules"}`}
	ch := &fakeChatter{responses: []ai.Response{{ToolCalls: []ai.ToolCall{call}}}}
	run := &fakeRunner{out: "should not run"}
	gd := &fakeGuard{verdict: "deny", reason: "递归/强制删除"}
	rec := newRecorder()
	a := newTestAgent(ch, run, gd, rec)

	if err := a.Turn(context.Background(), "删掉 node_modules"); err != nil {
		t.Fatalf("Turn: %v", err)
	}
	if len(run.calls) != 0 {
		t.Fatalf("a denied call reached the runner: %v", run.calls)
	}
	if len(rec.entries) != 1 {
		t.Fatalf("audit entries = %d, want 1 (denied calls are recorded)", len(rec.entries))
	}
	entry := rec.entries[0]
	if entry.OK {
		t.Fatal("denied call must be audited as not ok")
	}
	if !strings.HasPrefix(entry.Guard, "denied:") {
		t.Fatalf("guard field = %q, want denied:…", entry.Guard)
	}
	foundFinish := false
	for _, e := range rec.events {
		if e.Kind == EventToolFinished {
			foundFinish = true
			if e.OK || !strings.Contains(e.Out, "guardrail") {
				t.Fatalf("finish event = %+v", e)
			}
		}
	}
	if !foundFinish {
		t.Fatal("missing tool_finished event")
	}
}

func TestToolBudgetStopsTurn(t *testing.T) {
	calls := []ai.ToolCall{
		{ID: "a", Name: "list_sessions", Args: `{}`},
		{ID: "b", Name: "list_shells", Args: `{}`},
	}
	ch := &fakeChatter{responses: []ai.Response{{ToolCalls: calls}}}
	run := &fakeRunner{out: "[]"}
	gd := &fakeGuard{}
	rec := newRecorder()
	a := newTestAgent(ch, run, gd, rec, func(o *Options) { o.MaxToolCalls = 1 })

	if err := a.Turn(context.Background(), "列一下"); err != nil {
		t.Fatalf("Turn: %v", err)
	}
	if len(run.calls) != 1 {
		t.Fatalf("runner calls = %d, want 1 (budget is 1)", len(run.calls))
	}
	last := rec.events[len(rec.events)-1]
	if last.Kind != EventTurnDone || last.Reason != "budget" {
		t.Fatalf("last event = %+v, want turn_done budget", last)
	}
	if len(ch.requests) != 1 {
		t.Fatalf("chat calls = %d, want 1 (no further round after budget)", len(ch.requests))
	}
}

func TestTokenLimitStopsFurtherTools(t *testing.T) {
	call := ai.ToolCall{ID: "a", Name: "list_sessions", Args: `{}`}
	ch := &fakeChatter{responses: []ai.Response{
		{ToolCalls: []ai.ToolCall{call}, Usage: ai.Usage{TotalTokens: 200_000}},
	}}
	run := &fakeRunner{out: "[]"}
	rec := newRecorder()
	a := newTestAgent(ch, run, &fakeGuard{}, rec,
		func(o *Options) { o.SessionTokenLimit = 200_000 })

	if err := a.Turn(context.Background(), "继续"); err != nil {
		t.Fatalf("Turn: %v", err)
	}
	if len(run.calls) != 0 {
		t.Fatal("tools must not run once the session token limit is reached")
	}
	last := rec.events[len(rec.events)-1]
	if last.Reason != "tokens" {
		t.Fatalf("reason = %q, want tokens", last.Reason)
	}
}

func TestChatErrorReports(t *testing.T) {
	ch := &fakeChatter{errs: []error{errors.New("connect refused")}}
	rec := newRecorder()
	a := newTestAgent(ch, &fakeRunner{}, &fakeGuard{}, rec)

	err := a.Turn(context.Background(), "hi")
	if err == nil {
		t.Fatal("expected an error")
	}
	found := false
	for _, e := range rec.events {
		if e.Kind == EventError && strings.Contains(e.Err, "connect refused") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing error event: %v", rec.events)
	}
}

func TestCanceledTurn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rec := newRecorder()
	a := newTestAgent(&fakeChatter{}, &fakeRunner{}, &fakeGuard{}, rec)

	if err := a.Turn(ctx, "hi"); err == nil {
		t.Fatal("a canceled turn should return the context error")
	}
	last := rec.events[len(rec.events)-1]
	if last.Kind != EventTurnDone || last.Reason != "canceled" {
		t.Fatalf("last event = %+v, want canceled", last)
	}
}

func TestSummarize(t *testing.T) {
	if got := Summarize("  a\n b \t c ", 0); got != "a b c" {
		t.Fatalf("Summarize = %q", got)
	}
	if got := Summarize(strings.Repeat("x", 100), 10); got != strings.Repeat("x", 10)+"…" {
		t.Fatalf("Summarize trim = %q", got)
	}
}
