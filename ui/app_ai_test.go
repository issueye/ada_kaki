package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"adakaki/internal/ai"
	"adakaki/internal/audit"
	"adakaki/internal/guard"
	"adakaki/internal/store"

	myui "github.com/egoist/mygo/ui"
)

// The tool scope from the AI page gates the agent before the guardrail.
func TestAIScopeAllows(t *testing.T) {
	cases := []struct {
		scope string
		tool  string
		want  bool
	}{
		{"all", "run_command", true},
		{"all", "ssh_connect", true},
		{"readonly", "list_sessions", true},
		{"readonly", "read_session", true},
		{"readonly", "screen_session", true},
		{"readonly", "run_command", false},
		{"readonly", "write_session", false},
		{"readonly", "create_session", false},
		{"write", "run_command", true},
		{"write", "write_session", true},
		{"write", "list_sessions", true},
		{"write", "create_session", false},
		{"write", "ssh_connect", false},
		{"write", "close_session", false},
	}
	for _, tc := range cases {
		a := newAITestApp(t)
		a.store.Settings.AI.ToolScope = tc.scope
		if got := a.aiScopeAllows(tc.tool); got != tc.want {
			t.Fatalf("scope %q tool %q = %v, want %v", tc.scope, tc.tool, got, tc.want)
		}
	}
}

// Secret-looking JSON fields are blanked before the audit write, the rest of
// the arguments stay readable.
func TestRedactAIArgs(t *testing.T) {
	in := `{"host":"build","password":"hunter2","passphrase":"x","command":"go test"}`
	out := redactAIArgs(in)
	if strings.Contains(out, "hunter2") || strings.Contains(out, `"x"`) {
		t.Fatalf("secret values survived redaction: %s", out)
	}
	if !strings.Contains(out, "go test") || !strings.Contains(out, "build") {
		t.Fatalf("redaction dropped the readable fields: %s", out)
	}
	if got := redactAIArgs("not json"); got != "not json" {
		t.Fatalf("non-JSON args should pass through, got %q", got)
	}
}

func TestAIKeyState(t *testing.T) {
	a := newAITestApp(t)
	a.store.Settings.AI.APIKeyEnv = "ADAKAKI_TEST_KEY"
	label, set := a.aiKeyState()
	if set || !strings.Contains(label, "未设置") {
		t.Fatalf("without the variable: %q set=%v", label, set)
	}
	t.Setenv("ADAKAKI_TEST_KEY", "sk-test")
	label, set = a.aiKeyState()
	if !set || !strings.Contains(label, "已设置") {
		t.Fatalf("with the variable: %q set=%v", label, set)
	}
}

// A turn with no key fails visibly and never starts running.
func TestStartAITurnWithoutKey(t *testing.T) {
	a := newAITestApp(t)
	a.store.Settings.AI.Enabled = true
	a.store.Settings.AI.APIKeyEnv = "ADAKAKI_MISSING_KEY"
	a.aiInput = "跑一遍测试"

	a.startAITurn()

	state, errText, _, _, running := a.aiSnapshot()
	if running {
		t.Fatal("a turn without a key must not run")
	}
	if state != "error" || !strings.Contains(errText, "ADAKAKI_MISSING_KEY") {
		t.Fatalf("state = %q err = %q, want an error naming the variable", state, errText)
	}
	if a.aiInput != "跑一遍测试" {
		t.Fatalf("the draft should be kept, got %q", a.aiInput)
	}
}

// A turn while the operator is disabled changes nothing but the toast.
func TestStartAITurnDisabled(t *testing.T) {
	a := newAITestApp(t)
	a.store.Settings.AI.Enabled = false
	a.aiInput = "hi"

	a.startAITurn()

	state, _, _, _, running := a.aiSnapshot()
	if running || state != "idle" {
		t.Fatalf("state = %q running=%v, want idle and not running", state, running)
	}
	if !strings.Contains(a.pendingToast, "未启用") {
		t.Fatalf("toast = %q, want a disabled-operator hint", a.pendingToast)
	}
}

// While a turn runs the dock shows the live step instead of an empty state.
func TestAIDockShowsRunningStep(t *testing.T) {
	a := newAITestApp(t)
	a.store.Settings.AI.Enabled = true
	a.aiMu.Lock()
	a.aiState = "thinking"
	a.aiOps = []aiOp{{kind: "op", tool: "run_command", cmd: "go test", out: "ok", ok: true, ms: 12}}
	a.aiMu.Unlock()

	tt := myui.NewTester(func(c *myui.Context) { a.aiDock(c) }, 360, 720)
	tt.Frame()

	for _, want := range []string{"run_command", "go test", "正在思考", "查看审计", "1 步"} {
		if !tt.HasText(want) {
			t.Fatalf("dock does not show %q; texts = %v", want, tt.Texts())
		}
	}
}

// An end-to-end turn through the app runtime: settings -> key -> provider
// (a fake JSON endpoint) -> agent -> tool layer -> audit -> dock snapshot.
// It covers U4 (a tool card appears), U8's counting and U10's path, without
// a real provider or GUI.
func TestStartAITurnEndToEnd(t *testing.T) {
	a := newAITestApp(t)
	a.guard = guard.NewPolicy()

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			fmt.Fprint(w, `{"choices":[{"message":{"content":"","tool_calls":[{"id":"c1","function":{"name":"list_sessions","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13}}`)
			return
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"当前没有会话"},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":4,"total_tokens":16}}`)
	}))
	defer srv.Close()

	aiCfg := &a.store.Settings.AI
	aiCfg.Enabled = true
	aiCfg.Provider = store.AIProviderLocal
	aiCfg.BaseURL = srv.URL
	aiCfg.Model = "test-model"
	aiCfg.APIKeyEnv = "ADAKAKI_E2E_KEY"
	t.Setenv("ADAKAKI_E2E_KEY", "sk-test")

	log, err := audit.Open(filepath.Join(t.TempDir(), "audit.log"))
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	t.Cleanup(func() { _ = log.Close() })
	a.auditLog = log

	a.aiClient = func(p ai.Provider, key string) *ai.Client {
		return ai.NewClient(p, key, nil)
	}
	a.aiInput = "列一下会话"
	a.startAITurn()

	waitFor(t, 10*time.Second, func() bool {
		_, _, _, _, running := a.aiSnapshot()
		return !running
	})

	state, errText, tokens, ops, _ := a.aiSnapshot()
	if state != "done" {
		t.Fatalf("state = %q err = %q, want done", state, errText)
	}
	if tokens != 29 {
		t.Fatalf("tokens = %d, want 29 (13+16)", tokens)
	}
	var sawTool, sawMessage bool
	for _, op := range ops {
		if op.kind == "op" && op.tool == "list_sessions" && op.ok {
			sawTool = true
		}
		if op.kind == "msg" && strings.Contains(op.out, "没有会话") {
			sawMessage = true
		}
	}
	if !sawTool {
		t.Fatalf("no tool card for list_sessions; ops = %+v", ops)
	}
	if !sawMessage {
		t.Fatalf("no assistant message card; ops = %+v", ops)
	}

	entries, err := log.Tail(5)
	if err != nil {
		t.Fatalf("audit tail: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.Transport != "inapp" || e.Client != "adakaki-ai" || e.Tool != "list_sessions" || !e.OK {
		t.Fatalf("audit entry = %+v", e)
	}
	if strings.Count(fmt.Sprint(ops), "list_sessions") < 1 {
		t.Fatal("timeline lost the tool name")
	}
}
