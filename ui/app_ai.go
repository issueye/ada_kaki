// The AI operator's app-side runtime: the provider built from settings, the
// turn that drives agent.Agent, the guard/scope adapter, the audit hook and
// the connection probe. The dock reads state through aiSnapshot so the
// agent goroutine and the UI thread never touch the same fields unlocked.

package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"adakaki/internal/agent"
	"adakaki/internal/ai"
	"adakaki/internal/audit"
	"adakaki/internal/guard"
	"adakaki/internal/tools"
)

// aiTurnTimeout bounds one turn; a turn that runs this long is stopped.
const aiTurnTimeout = 5 * time.Minute

// aiProvider builds the provider the settings describe.
func (a *app) aiProvider() ai.Provider {
	cfg := a.store.Settings.AI
	return ai.Provider{
		Name:    aiLabel(cfg.Provider, aiProviderOptions),
		BaseURL: strings.TrimSpace(cfg.BaseURL),
		Model:   strings.TrimSpace(cfg.Model),
		KeyEnv:  strings.TrimSpace(cfg.APIKeyEnv),
	}
}

// probeAI asks the configured provider for a one-token answer, off the UI
// thread, and reports the outcome beside the button.
func (a *app) probeAI() {
	if a.aiProbe == "测试中…" {
		return
	}
	a.aiProbe = "测试中…"
	a.aiProbeOK = false
	a.refreshSettingsWindow()

	provider := a.aiProvider()
	envName := provider.KeyEnv

	go func() {
		key, err := ai.KeyFromEnv(envName)
		if err != nil {
			a.finishProbe(false, err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		client := ai.NewClient(provider, key, nil)
		start := time.Now()
		resp, err := client.Chat(ctx, ai.Request{
			Messages:  []ai.Message{{Role: "user", Content: "ping"}},
			MaxTokens: 1,
		}, nil)
		if err != nil {
			a.finishProbe(false, err.Error())
			return
		}
		elapsed := time.Since(start).Round(time.Millisecond)
		msg := "连接成功 · " + provider.Model + " · " + elapsed.String()
		if resp.Usage.Total() > 0 {
			msg += fmt.Sprintf(" · %d tokens", resp.Usage.Total())
		}
		a.finishProbe(true, msg)
	}()
}

// finishProbe publishes the outcome and repaints the settings window.
func (a *app) finishProbe(ok bool, msg string) {
	a.aiProbe = msg
	a.aiProbeOK = ok
	a.refreshSettingsWindow()
}

// refreshSettingsWindow redraws the settings window from any goroutine.
func (a *app) refreshSettingsWindow() {
	w := a.settingsWin
	if w == nil {
		return
	}
	w.Update(func() {})
}

// aiSnapshot copies the runtime state the dock renders.
func (a *app) aiSnapshot() (state, errText string, tokens int, ops []aiOp, running bool) {
	a.aiMu.Lock()
	defer a.aiMu.Unlock()
	ops = make([]aiOp, len(a.aiOps))
	copy(ops, a.aiOps)
	return a.aiState, a.aiErr, a.aiTokenTotal, ops, a.aiRunning
}

// aiStepCount is how many tool cards the badge shows.
func (a *app) aiStepCount() int {
	a.aiMu.Lock()
	defer a.aiMu.Unlock()
	n := 0
	for _, op := range a.aiOps {
		if op.kind == "op" {
			n++
		}
	}
	return n
}

// startAITurn runs one turn with the current input; it never blocks the UI.
func (a *app) startAITurn() {
	prompt := strings.TrimSpace(a.aiInput)
	if prompt == "" {
		return
	}
	if !a.store.Settings.AI.Enabled {
		a.pendingToast = "AI 操作员未启用：设置 → AI 打开开关"
		a.refreshUI()
		return
	}
	if a.aiRunning {
		a.pendingToast = "上一轮还在执行，等它结束再发"
		a.refreshUI()
		return
	}

	provider := a.aiProvider()
	cfg := a.store.Settings.AI
	key, err := ai.KeyFromEnv(provider.KeyEnv)
	if err != nil {
		a.aiMu.Lock()
		a.aiState = "error"
		a.aiErr = err.Error()
		a.aiRunning = false
		a.aiMu.Unlock()
		a.pendingToast = err.Error()
		a.refreshUI()
		return
	}

	client := ai.NewClient(provider, key, nil)
	ag := a.aiAgent
	if ag == nil {
		ag = agent.New(agent.Options{
			Chatter:           client,
			Runner:            aiRunner{backend: a.toolBackend},
			Guard:             aiGuard{a: a},
			MaxToolCalls:      cfg.MaxToolCalls,
			SessionTokenLimit: cfg.SessionTokenLimit,
			Session:           a.activeID,
			Reasoning:         cfg.Reasoning,
			Emit:              a.aiEmit,
			Audit:             a.aiAudit,
		})
		a.aiAgent = ag
	} else {
		ag.Configure(cfg.MaxToolCalls, cfg.SessionTokenLimit, cfg.Reasoning)
		ag.SetSession(a.activeID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), aiTurnTimeout)
	a.aiMu.Lock()
	a.aiRunning = true
	a.aiCancel = cancel
	a.aiInput = ""
	a.aiOps = nil
	a.aiState = "thinking"
	a.aiErr = ""
	a.aiMu.Unlock()
	a.refreshUI()

	go func() {
		turnErr := ag.Turn(ctx, prompt)
		cancel()
		a.aiMu.Lock()
		a.aiRunning = false
		a.aiCancel = nil
		switch {
		case turnErr != nil:
			a.aiState = "error"
			a.aiErr = turnErr.Error()
		case a.aiState != "error":
			a.aiState = "done"
		}
		a.aiMu.Unlock()
		a.refreshUI()
	}()
}

// cancelAITurn stops the running turn, if any.
func (a *app) cancelAITurn() {
	a.aiMu.Lock()
	cancel := a.aiCancel
	a.aiMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// aiEmit turns agent events into dock state; it runs on the agent goroutine.
func (a *app) aiEmit(ev agent.Event) {
	a.aiMu.Lock()
	switch ev.Kind {
	case agent.EventThinking:
		a.aiState = "thinking"
	case agent.EventAssistant:
		a.aiOps = append(a.aiOps, aiOp{kind: "msg", out: ev.Out})
	case agent.EventToolStarted:
		a.aiState = "tool"
	case agent.EventToolFinished:
		a.aiState = "thinking"
		a.aiOps = append(a.aiOps, aiOp{
			kind: "op", tool: ev.Tool, cmd: ev.Cmd,
			out: agent.Summarize(ev.Out, 90), ok: ev.OK, ms: ev.MS, err: ev.Err,
		})
	case agent.EventTurnDone:
		a.aiTokenTotal = ev.Tokens
		switch ev.Reason {
		case "done", "":
			a.aiState = "done"
		case "budget":
			a.aiState = "done"
			a.aiErr = fmt.Sprintf("已达单轮工具上限（%d），需要继续请再发一句", a.store.Settings.AI.MaxToolCalls)
		case "tokens":
			a.aiState = "done"
			a.aiErr = "会话 token 已到上限，建议开一个新会话"
		case "canceled":
			a.aiState = "done"
			a.aiErr = "本轮已取消"
		}
	case agent.EventError:
		a.aiState = "error"
		a.aiErr = ev.Err
	}
	a.aiMu.Unlock()
	a.refreshUI()
}

// aiAudit records the agent's own calls, tagged so the audit window can tell
// them from external MCP clients.
func (a *app) aiAudit(entry audit.Entry) {
	if a.auditLog == nil || !a.store.Settings.AI.AuditOn() {
		return
	}
	if a.store.Settings.AI.AuditRedactOn() {
		entry.Args = redactAIArgs(entry.Args)
	}
	_ = a.auditLog.Record(entry)
}

// redactAIArgs blanks secret-looking JSON fields before they reach disk.
func redactAIArgs(raw string) string {
	var fields map[string]any
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return raw
	}
	for key := range fields {
		if secretKeyName(key) {
			fields[key] = "[redacted]"
		}
	}
	out, err := json.Marshal(fields)
	if err != nil {
		return raw
	}
	return string(out)
}

func secretKeyName(key string) bool {
	k := strings.ToLower(key)
	for _, needle := range []string{"password", "passphrase", "secret", "token", "api_key", "apikey"} {
		if strings.Contains(k, needle) {
			return true
		}
	}
	return false
}

// aiScopeAllows applies the settings' tool scope before the guardrail runs.
func (a *app) aiScopeAllows(tool string) bool {
	switch a.store.Settings.AI.ToolScope {
	case "readonly":
		switch tool {
		case "list_shells", "list_sessions", "list_ssh_profiles", "read_session", "screen_session":
			return true
		}
		return false
	case "write":
		switch tool {
		case "create_session", "close_session", "reconnect_session", "ssh_connect":
			return false
		}
		return true
	default:
		return true
	}
}

// aiGuard adapts the console's policy to the agent's Guard interface.
type aiGuard struct{ a *app }

func (g aiGuard) Check(tool, command string) (string, string) {
	if !g.a.aiScopeAllows(tool) {
		return "deny", "工具范围限制（设置 → AI → 允许工具）"
	}
	v, rule := g.a.guard.Check(guard.Request{
		Tool: tool, Client: "adakaki-ai", Session: g.a.activeID, Command: command,
	})
	if rule != nil {
		return string(v), rule.Reason
	}
	return string(v), ""
}

// aiRunner adapts the shared tool backend to the agent's Runner interface.
type aiRunner struct{ backend *tools.Backend }

func (r aiRunner) Call(name, args string) (string, error) {
	if r.backend == nil {
		return "", fmt.Errorf("no tool backend")
	}
	return r.backend.Call(name, args)
}

// aiApplyGuardMode pushes the AI page's strictness into the policy.
func (a *app) aiApplyGuardMode() {
	a.guard.SetMode(guard.ModeFromString(a.store.Settings.AI.GuardMode))
}

// aiKeyState is the credential line the settings page shows.
func (a *app) aiKeyState() (string, bool) {
	env := a.store.Settings.AI.APIKeyEnv
	if env == "" {
		return "未设置", false
	}
	if os.Getenv(env) == "" {
		return env + " · 未设置", false
	}
	return env + " · 已设置", true
}
