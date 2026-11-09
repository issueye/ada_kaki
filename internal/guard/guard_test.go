package guard

import (
	"testing"
)

func TestDefaultRulesMatch(t *testing.T) {
	p := NewPolicy()
	cases := []struct {
		command string
		verdict Verdict
	}{
		{"git status", Allow},
		{"dir /b", Allow},
		{"ls -la", Allow},
		{"npm install", Allow},
		{"rm file.txt", Allow},
		{"rm -f build.log", Allow}, // a single forced delete is not the rail's business
		{"rm -rf node_modules", Denied},
		{"rm -r old", Denied},
		{"sudo rm -rf /x", Denied},
		{"del /s /q build", Denied},
		{"rmdir /s data", Denied},
		{"Remove-Item -Recurse -Force C:\\tmp", Denied},
		{"format c:", Denied},
		{"FORMAT C: /q", Denied},
		{"dd if=zero of=/dev/sda", Denied},
		{"mkfs.ext4 /dev/sdb1", Denied},
		{"shutdown /s /t 0", Denied},
		{"Stop-Computer -Force", Denied},
		{"git push --force origin main", Denied},
		{"git reset --hard HEAD~1", Denied},
		{"chmod -R 777 /var", Denied},
		{"reg add HKLM\\SOFTWARE\\x", Denied},
		{"echo hello && rm -rf ./build", Denied}, // inside a compound too
	}
	for _, c := range cases {
		v, rule := p.Check(Request{Tool: "run_command", Command: c.command})
		if v != c.verdict {
			reason := ""
			if rule != nil {
				reason = rule.Reason
			}
			t.Errorf("%q → %v (rule %q); want %v", c.command, v, reason, c.verdict)
		}
	}
}

func TestConfirmNeedsHuman(t *testing.T) {
	p := NewPolicy()
	// No human (headless), default deny:
	v, _ := p.Check(Request{Tool: "run_command", Command: "rm -rf /tmp/x"})
	if v != Denied {
		t.Errorf("headless confirm = %v; want deny", v)
	}
	// Headless allow per setting:
	p.HeadlessConfirm = "allow"
	v, _ = p.Check(Request{Tool: "run_command", Command: "rm -rf /tmp/x"})
	if v != ConfirmOK {
		t.Errorf("headless allow = %v; want confirm_ok", v)
	}
	// A human says no, then yes:
	p.HeadlessConfirm = ""
	answer := false
	p.Confirm = func(req Request, rule Rule) bool { return answer }
	if v, _ := p.Check(Request{Tool: "run_command", Command: "rm -rf /tmp/x"}); v != Denied {
		t.Error("human refusal must deny")
	}
	answer = true
	if v, _ := p.Check(Request{Tool: "run_command", Command: "rm -rf /tmp/x"}); v != ConfirmOK {
		t.Error("human approval must confirm_ok")
	}
}

func TestCustomRulesAndDisable(t *testing.T) {
	p := FromSettings(false, "", []string{`(?i)terraform\s+destroy`}, []string{`(?i)docker\s+system\s+prune`})
	if v, rule := p.Check(Request{Tool: "run_command", Command: "terraform destroy -auto-approve"}); v != Denied || rule == nil || !rule.Custom {
		t.Errorf("custom deny = %v, %v", v, rule)
	}
	if v, _ := p.Check(Request{Tool: "run_command", Command: "docker system prune -a"}); v != Denied {
		t.Errorf("custom confirm without human = %v; want deny", v)
	}
	if err := p.AddCustom(Deny, "(unclosed", "bad"); err == nil {
		t.Error("a broken custom pattern must be reported")
	}
	builtin, custom := p.Count()
	if custom != 2 || builtin < 10 {
		t.Errorf("counts = %d builtin, %d custom", builtin, custom)
	}
	p.ClearCustom()
	if _, custom := p.Count(); custom != 0 {
		t.Error("ClearCustom left custom rules")
	}
	// Everything off:
	p.Enabled = false
	if v, _ := p.Check(Request{Tool: "run_command", Command: "rm -rf /"}); v != Allow {
		t.Error("a disabled policy allows everything")
	}
}
