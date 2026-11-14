// The session and new-session menus, with the labels and state mark
// they share with the rest of the window.

package ui

import (
	"adakaki/internal/engine"
	"adakaki/internal/shells"
	"adakaki/internal/store"
	"fmt"

	myui "github.com/egoist/mygo/ui"
)

// newSessionMenu is the 新建 button's menu, shortcuts annotated.
func (a *app) newSessionMenu(m *myui.Menu) {
	if m.Item("新建终端").Shortcut(myui.Cmd|myui.Shift, myui.KeyT).Chosen() {
		a.newSession(a.defaultShellID(), "", "")
	}
	m.Separator()
	for _, sh := range a.shells {
		if sh.Found {
			if m.Item(sh.Name).Chosen() {
				a.newSession(sh.ID, "", "")
			}
		}
	}
	m.Separator()
	if m.Item("SSH 连接…").Chosen() {
		a.showSSH = true
	}
	if m.Item("自定义命令…").Chosen() {
		a.showCustom = true
	}
}

// authLabel names an SSH auth kind.
func authLabel(kind string) string {
	switch kind {
	case "key":
		return "私钥文件"
	case "agent":
		return "OpenSSH Agent"
	default:
		return "密码"
	}
}

// strictLabel names a host key verification mode.
func strictLabel(v string) string {
	switch v {
	case "accept_new":
		return "校验：自动信任新主机"
	case "off":
		return "校验：关闭（不安全）"
	default:
		return "校验：首次连接弹窗确认"
	}
}

// defaultShellLabel names the default shell setting.
func (a *app) defaultShellLabel() string {
	if id := shells.ID(a.store.Settings.DefaultShell); id != "" {
		if sh, ok := shells.Get(id); ok {
			return sh.Name
		}
	}
	return "自动（第一个可用）"
}

// fillSSH puts a saved profile into the SSH dialog's fields.
func (a *app) fillSSH(p store.Profile) {
	a.sshProfile = p.Name
	a.sshHost = p.Host
	a.sshPort = ""
	if p.Port != 0 {
		a.sshPort = fmt.Sprint(p.Port)
	}
	a.sshUser = p.User
	a.sshAuth = p.AuthKind
	if a.sshAuth == "" {
		a.sshAuth = "password"
	}
	a.sshKeyPath = p.KeyPath
	// The saved secret (a key passphrase) fills the pass field; a saved
	// password is not shown, and is used when the field stays empty.
	a.sshPass = ""
	if p.AuthKind == "key" {
		if plain, err := store.Unprotect(p.Secret); err == nil {
			a.sshPass = plain
		}
	}
}

// sessionMenu is a session's right-click menu, in the sidebar and tabs.
// Splitting acts on the pane the session lives in, so the menu carries
// it wherever the right click landed. Closing is refused while an agent
// operates the session; the mirror window stays welcome.
func (a *app) sessionMenu(m *myui.Menu, s *engine.Session) {
	if leaf := a.leafByID(s.ID()); leaf != nil {
		if m.Item("向右分屏").Chosen() {
			a.splitSession(s.ID(), true)
		}
		if m.Item("向下分屏").Chosen() {
			a.splitSession(s.ID(), false)
		}
		m.Separator()
	}
	locked, _ := s.AgentLock()
	if a.popouts[s.ID()] != nil {
		if m.Item("关闭同屏窗口").Chosen() {
			a.recall(s)
		}
	} else if m.Item("弹出同屏窗口").Chosen() {
		a.popout(s)
	}
	if s.State() == engine.Exited {
		if m.Item("重新连接").Chosen() {
			a.reconnect(s)
		}
	}
	m.Separator()
	label := "关闭会话"
	if s.State() == engine.Exited {
		label = "移除"
	}
	if locked {
		label += "（Agent 操作中）"
	}
	if m.Item(label).Disabled(locked).Chosen() {
		a.closeSession(s)
	}
	if s.State() != engine.Exited {
		m.Separator()
		force := "强制关闭会话"
		if locked {
			force += "（Agent 操作中）"
		}
		if m.Item(force).Disabled(locked).Danger().
			Shortcut(myui.Cmd|myui.Shift, myui.KeyF12).Chosen() {
			a.forceCloseSession(s)
		}
	}
}

// stateMark is the little dot that tells a session's state, in the
// theme's own colors; the bot stands for a session an AI agent operates.
func (a *app) stateMark(c *myui.Context, s *engine.Session) {
	t := c.Theme()
	if on, _ := s.AgentLock(); on {
		myui.Icon(c, icoBot).FontSize(12).TextColor(t.Warning)
		return
	}
	color := t.Success
	switch s.State() {
	case engine.Exited:
		color = t.TextMuted
	case engine.Starting:
		color = t.Warning
	}
	dot(c, color, 6)
}

// agentSummary counts the agent's sessions and names the tool and target
// of the first one, for the sidebar card and the title strip.
func (a *app) agentSummary() (int, string, string) {
	n := 0
	tool, target := "", ""
	for _, s := range a.mgr.List() {
		if on, t := s.AgentLock(); on {
			n++
			if tool == "" {
				tool, target = t, s.Title()
			}
		}
	}
	return n, tool, target
}
