// Package guard is the console's safety rail for AI-written commands:
// a rule base of dangerous patterns, each at a level — deny outright, or
// confirm with a human before the command runs. Denied and confirmed
// calls alike land in the audit trail.
package guard

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// Level of a rule.
type Level string

const (
	// Deny refuses the call outright.
	Deny Level = "deny"
	// Confirm asks a human; without one (headless), the setting decides.
	Confirm Level = "confirm"
)

// Rule is one dangerous pattern.
type Rule struct {
	Level    Level
	Pattern  *regexp.Regexp
	Source   string // the pattern text, for display
	Reason   string
	Custom   bool // user-configured
}

// Policy is the rule base plus the human-confirm channel.
type Policy struct {
	mu     sync.Mutex
	rules  []Rule
	// Confirm asks the user (GUI): it runs on the tool-call goroutine and
	// may block until the user answers. nil means there is no human to
	// ask — headless — and Confirm calls follow HeadlessConfirm.
	Confirm func(req Request, rule Rule) bool
	// HeadlessConfirm decides Confirm-level calls without a human:
	// "deny" (the safe default) or "allow".
	HeadlessConfirm string
	// Enabled turns the whole rail off.
	Enabled bool
}

// Request is one call the policy may judge.
type Request struct {
	Tool    string
	Client  string
	Session string
	Command string // what would run: run_command's command, or write's last line
}

// Verdict of a check.
type Verdict string

const (
	Allow  Verdict = "allow"
	// ConfirmOK means a human approved a Confirm-level call.
	ConfirmOK Verdict = "confirm_ok"
	Denied    Verdict = "deny"
)

// DefaultRules is the shipped rule base, Windows and Unix alike (the
// console runs git-bash too). Deliberately conservative: these hit what
// destroys data or machines, not everyday work.
func DefaultRules() []Rule {
	defs := []struct {
		level  Level
		pattern string
		reason string
	}{
		{Deny, `(?i)\bformat(\.com)?\s+[a-z]:`, "格式化磁盘"},
		{Deny, `(?i)\bdiskpart\b`, "磁盘分区工具"},
		{Deny, `(?i)\bdd\s+[^|]*\bof=/dev/`, "直接写入块设备"},
		{Deny, `(?i)\bmkfs(\.\w+)?\b`, "建立文件系统（覆盖设备）"},
		{Confirm, `(?i)(^|[\s;&|])rm\s+(-\w+\s+)*-\w*[rs]`, "递归/强制删除"},
		{Confirm, `(?i)(^|[\s;&|])(del|rd|rmdir)\s+/\w*[sq]`, "递归删除目录/文件"},
		{Confirm, `(?i)Remove-Item\s+(-\w+\s+)*-Recurse`, "PowerShell 递归删除"},
		{Confirm, `(?i)\b(shutdown|logoff)(\.\w+)?\s+(/|\-\w)`, "关机/注销"},
		{Confirm, `(?i)(Stop|Restart)-Computer\b`, "PowerShell 关机/重启"},
		{Confirm, `(?i)\bgit\s+push\b[^|]*--force`, "强制推送覆盖远端"},
		{Confirm, `(?i)\bgit\s+reset\s+--hard\b`, "丢弃全部本地改动"},
		{Confirm, `(?i)\bchmod\s+(-R\s+)?777\b`, "全开放权限"},
		{Confirm, `(?i)\breg\s+add\s+HKLM`, "写系统注册表"},
		{Confirm, `(?i):\(\)\s*\{\s*:\s*\|\s*:\s*&\s*\}\s*;?\s*:` , "fork 炸弹"},
	}
	out := make([]Rule, 0, len(defs))
	for _, d := range defs {
		re, err := regexp.Compile(d.pattern)
		if err != nil {
			continue // a bad built-in rule is a bug; skip it, never fail
		}
		out = append(out, Rule{Level: d.level, Pattern: re, Source: d.pattern, Reason: d.reason})
	}
	return out
}

// NewPolicy returns the shipped rule base.
func NewPolicy() *Policy {
	return &Policy{rules: DefaultRules(), Enabled: true, HeadlessConfirm: "deny"}
}

// AddCustom compiles and adds a user rule; it reports the error for the
// settings UI.
func (p *Policy) AddCustom(level Level, pattern, reason string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("guard: 规则 %q: %w", pattern, err)
	}
	p.mu.Lock()
	p.rules = append(p.rules, Rule{Level: level, Pattern: re, Source: pattern, Reason: reason, Custom: true})
	p.mu.Unlock()
	return nil
}

// ClearCustom drops every user rule, as the settings reload does.
func (p *Policy) ClearCustom() {
	p.mu.Lock()
	keep := p.rules[:0]
	for _, r := range p.rules {
		if !r.Custom {
			keep = append(keep, r)
		}
	}
	p.rules = keep
	p.mu.Unlock()
}

// FromSettings builds a policy from the console's settings: built-in
// rules plus the user's own, the enable switch, and the headless
// Confirm decision.
func FromSettings(disabled bool, headless string, deny, confirm []string) *Policy {
	p := NewPolicy()
	p.Enabled = !disabled
	if headless != "" {
		p.HeadlessConfirm = headless
	}
	for _, pat := range deny {
		_ = p.AddCustom(Deny, pat, "自定义 deny 规则")
	}
	for _, pat := range confirm {
		_ = p.AddCustom(Confirm, pat, "自定义 confirm 规则")
	}
	return p
}

// Count returns how many rules are in force, built-in and custom.
func (p *Policy) Count() (builtin, custom int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, r := range p.rules {
		if r.Custom {
			custom++
		} else {
			builtin++
		}
	}
	return builtin, custom
}

// Check judges a request. A Confirm verdict without a human follows
// HeadlessConfirm; a human's answer comes back as ConfirmOK.
func (p *Policy) Check(req Request) (Verdict, *Rule) {
	if !p.Enabled {
		return Allow, nil
	}
	command := strings.TrimSpace(req.Command)
	if command == "" {
		return Allow, nil
	}
	p.mu.Lock()
	rules := append([]Rule(nil), p.rules...)
	headless := p.HeadlessConfirm
	confirmFn := p.Confirm
	p.mu.Unlock()

	for _, r := range rules {
		if !r.Pattern.MatchString(command) {
			continue
		}
		switch r.Level {
		case Deny:
			return Denied, &r
		case Confirm:
			if confirmFn != nil {
				if confirmFn(req, r) {
					return ConfirmOK, &r
				}
				return Denied, &r
			}
			if strings.EqualFold(headless, "allow") {
				return ConfirmOK, &r
			}
			return Denied, &r
		}
	}
	return Allow, nil
}
