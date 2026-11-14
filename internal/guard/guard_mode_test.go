package guard

import "testing"

// The three modes from the AI settings page (docs/AI-功能规划与设计.md §F6):
// only the strictness changes, the rule base stays shared with MCP.

func TestModeHighOnlyAsksForDenyRulesOnly(t *testing.T) {
	p := NewPolicy()
	p.SetMode(ModeHighOnly)
	asked := 0
	p.Confirm = func(req Request, rule Rule) bool {
		asked++
		return true
	}

	v, rule := p.Check(Request{Command: "diskpart"})
	if v != ConfirmOK || rule == nil {
		t.Fatalf("diskpart (a deny rule) should ask and pass once confirmed, got %v %v", v, rule)
	}
	if asked != 1 {
		t.Fatalf("asked = %d, want 1", asked)
	}

	v, rule = p.Check(Request{Command: "rm -rf node_modules"})
	if v != Allow || rule != nil {
		t.Fatalf("a confirm-level command should pass silently in high_only, got %v %v", v, rule)
	}
	if asked != 1 {
		t.Fatalf("confirm-level match must not ask; asked = %d", asked)
	}
}

func TestModeHighOnlyDeniesWhenRefused(t *testing.T) {
	p := NewPolicy()
	p.SetMode(ModeHighOnly)
	p.Confirm = func(Request, Rule) bool { return false }

	if v, _ := p.Check(Request{Command: "mkfs.ext4 /dev/sdb1"}); v != Denied {
		t.Fatalf("refused deny-level call = %v, want deny", v)
	}
}

func TestModeConfirmAllUnchanged(t *testing.T) {
	p := NewPolicy()
	p.SetMode(ModeConfirmAll)
	asked := 0
	p.Confirm = func(Request, Rule) bool { asked++; return true }

	if v, _ := p.Check(Request{Command: "rm -rf node_modules"}); v != ConfirmOK {
		t.Fatalf("confirm-level call = %v, want confirm_ok", v)
	}
	if asked != 1 {
		t.Fatalf("confirm_all should ask once, asked = %d", asked)
	}

	if v, _ := p.Check(Request{Command: "diskpart"}); v != Denied {
		t.Fatalf("deny-level call = %v, want deny", v)
	}
	if asked != 1 {
		t.Fatalf("deny rules must not ask in confirm_all, asked = %d", asked)
	}
}

func TestModeAllowSkipsEverything(t *testing.T) {
	p := NewPolicy()
	p.SetMode(ModeAllow)
	p.Confirm = func(Request, Rule) bool {
		t.Fatal("allow mode must not ask")
		return false
	}
	for _, cmd := range []string{"rm -rf /", "diskpart", "mkfs.ext4 /dev/sda1"} {
		if v, _ := p.Check(Request{Command: cmd}); v != Allow {
			t.Fatalf("%q in allow mode = %v, want allow", cmd, v)
		}
	}
}

func TestModeFromString(t *testing.T) {
	cases := map[string]Mode{
		"confirm_all": ModeConfirmAll,
		"high_only":   ModeHighOnly,
		"allow":       ModeAllow,
		"":            ModeConfirmAll,
		"nonsense":    ModeConfirmAll,
	}
	for in, want := range cases {
		if got := ModeFromString(in); got != want {
			t.Fatalf("ModeFromString(%q) = %q, want %q", in, got, want)
		}
	}
}
