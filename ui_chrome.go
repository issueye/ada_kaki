package main

import (
	"fmt"
	"strings"
	"time"

	"adakaki/internal/audit"
	"adakaki/internal/engine"
	"adakaki/internal/shells"
	"adakaki/internal/store"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

// colorMask is the agent mask's translucent veil over a terminal.
var colorMask = ui.RGBA(8, 10, 18, 0.55)

// icoShield is the guard dialog's icon: a shield with a check.
var icoShield = iconSVG(`<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/><path d="m9 12 2 2 4-4"/>`)

// iconButton is an icon-only button, named and tipped: an icon alone is
// invisible to assistive technology.
func iconButton(c *ui.Context, svg *ui.SVG, name, tip string, fn func()) {
	if ui.Button(c, "").Label(name).Tooltip(tip).Padding(5, 8).Children(func() {
		ui.Icon(c, svg).FontSize(15)
	}).Clicked() {
		fn()
	}
}

// sectionLabel is a small heading that groups the rows of a dialog.
func sectionLabel(c *ui.Context, text string) {
	ui.Text(c, text).Bold().FontSize(12).TextColor(c.Theme().TextMuted)
}

// kbd draws a keyboard key the way web UIs do: a small bordered chip.
func kbd(c *ui.Context, key string) {
	t := c.Theme()
	ui.Box(c).Padding(2, 6).Radius(4).Border(1, t.Border).Background(t.Surface).Children(func() {
		ui.Text(c, key).FontSize(11)
	})
}

// titleStrip is a window's own title bar (TitleBarHidden): the title at
// left, extra controls at right, the whole strip dragging the window and
// keeping clear of the system's window controls.
func titleStrip(c *ui.Context, title string, right func()) {
	bar := c.TitleBar()
	ui.Row(c).Height(max(bar.Height, 38)).Padding(0, bar.Right+10, 0, 12).
		DragWindow().AlignItems(ui.Center).Gap(8).Children(func() {
		if title != "" {
			ui.Text(c, title).Bold().FontSize(13)
		}
		ui.Spacer(c)
		if right != nil {
			right()
		}
	})
}

// mainView builds the main window: a sidebar that runs to the top of the
// window beside the content pane — the title strip with the actions, the
// tab bar, the find bar, the terminal panes and the status bar — with
// the dialogs over them.
func (a *app) mainView(c *ui.Context) {
	if a.pendingToast != "" {
		c.Toast(a.pendingToast)
		a.pendingToast = ""
	}

	// ---- keyboard shortcuts (window level, ahead of the terminals) ----
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyT) {
		a.newSession(a.defaultShellID(), "", "")
	}
	if c.Shortcut(ui.Cmd, ui.KeyTab) {
		a.cycleSession(1)
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyTab) {
		a.cycleSession(-1)
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyW) {
		if s := a.active(); s != nil {
			a.closeSession(s)
		}
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyP) {
		if s := a.active(); s != nil {
			a.popout(s)
		}
	}
	if c.Shortcut(ui.Cmd, ui.KeyF) {
		a.openFind()
	}

	ui.Split(c, &a.sidebarWidth,
		func() { a.sidebarPane(c) },
		func() { a.contentPane(c) }).Fill()

	a.mainDialogs(c)
}

// sidebarPane is the left column: the app identity (dragging the window)
// over the sessions, running to the top of the window.
func (a *app) sidebarPane(c *ui.Context) {
	bar := c.TitleBar()
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		ui.Row(c).Height(max(bar.Bottom, 44)).Padding(0, 8, 0, bar.Left+12).
			DragWindow().AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Icon(c, icoTerminal).FontSize(16).TextColor(t.Accent)
			ui.Text(c, "AdaKaki").Bold().FontSize(14)
		})
		if ui.Sidebar(c, &a.activeID, func() {
			ui.SidebarSection(c, "本地 Shell", nil, func() {
				for _, s := range a.sessionsOf(engine.Local) {
					item := ui.SidebarItem(c.Key(s.ID()), s.ID(), icoTerminal, s.Title())
					item.Children(func() { a.stateMark(c, s) })
					item.ContextMenu(func(m *ui.Menu) { a.sessionMenu(m, s) })
				}
			})
			ui.SidebarSection(c, "SSH", nil, func() {
				for _, s := range a.sessionsOf(engine.SSH) {
					item := ui.SidebarItem(c.Key(s.ID()), s.ID(), icoGlobe, s.Title())
					item.Children(func() { a.stateMark(c, s) })
					item.ContextMenu(func(m *ui.Menu) { a.sessionMenu(m, s) })
				}
			})
		}).Fill().Changed() {
			a.viewSession(a.activeID)
		}
	})
}

// contentPane is the right column: the title strip (actions at the
// window controls' left), the tab bar, the find bar, the panes and the
// status bar.
func (a *app) contentPane(c *ui.Context) {
	bar := c.TitleBar()
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		// the title strip: draggable everywhere but the controls
		ui.Row(c).Height(max(bar.Height, 38)).Padding(0, bar.Right+10, 0, 10).
			DragWindow().AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Spacer(c)
			ui.Button(c, "").Label("新建会话").Children(func() {
				ui.Icon(c, icoPlus).FontSize(14)
				ui.Text(c, "新建")
			}).Menu(a.newSessionMenu)
			iconButton(c, icoHistory, "审计日志", "查看 MCP 调用审计", a.openAudit)
			iconButton(c, icoSettings, "设置", "打开设置", a.openSettings)
		})
		a.tabBar(c)
		if a.findOpen {
			a.findBar(c)
		}
		ui.Divider(c)
		a.terminalArea(c)
		a.statusBar(c)
	})
}

// statusBar is the one-line summary along the bottom: sessions, the
// agent at work, and the MCP endpoints.
func (a *app) statusBar(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Height(26).Padding(0, 12).Background(t.Surface).
		AlignItems(ui.Center).Gap(10).Children(func() {
		n := len(a.mgr.List())
		dot, dotColor := icoDot, t.Success
		if n == 0 {
			dotColor = t.TextMuted
		}
		ui.Icon(c, dot).FontSize(8).TextColor(dotColor)
		ui.Text(c, fmt.Sprintf("%d 个会话", n)).FontSize(11).TextColor(t.TextMuted)
		if s := a.active(); s != nil {
			ui.Text(c, s.Title()).FontSize(11).MaxWidth(220).Ellipsis("…")
		}
		ui.Spacer(c)
		if agents := a.agentBusy(); agents > 0 {
			ui.Icon(c, icoBot).FontSize(13).TextColor(t.Warning)
			ui.Text(c, fmt.Sprintf("Agent 运行中 ×%d", agents)).FontSize(11).TextColor(t.Warning)
			ui.Divider(c).Height(14)
		}
		port := a.store.Settings.MCPHTTPPort
		if port == 0 {
			port = 8731
		}
		if a.mcpUp {
			ui.Icon(c, icoDot).FontSize(8).TextColor(t.Success)
			ui.Text(c, fmt.Sprintf("MCP :%d", port)).FontSize(11).TextColor(t.TextMuted)
		} else {
			ui.Text(c, "MCP HTTP 关").FontSize(11).TextColor(t.TextMuted)
		}
		ui.Text(c, "·").FontSize(11).TextColor(t.TextMuted)
		ui.Text(c, "mcp stdio 可用").FontSize(11).TextColor(t.TextMuted)
	})
}

func (a *app) startCustom() {
	cmd := a.customCmd
	a.showCustom = false
	a.customCmd = ""
	if cmd != "" {
		a.newSession(shells.Custom, "", cmd)
	}
}

// mainDialogs is every dialog of the main window.
func (a *app) mainDialogs(c *ui.Context) {
	t := c.Theme()
	if a.showQuit {
		ui.Modal(c, &a.showQuit, func() {
			agents := a.agentBusy()
			ui.Column(c.Key("quit-dialog")).Width(380).Gap(12).Children(func() {
				ui.Text(c, "退出 AdaKaki？").FontSize(16).Bold()
				if agents > 0 {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Icon(c, icoAlert).TextColor(t.Danger)
						ui.Text(c, fmt.Sprintf("AI Agent 正在操作 %d 个会话，退出会中断它们", agents)).TextColor(t.Danger)
					})
				}
				ui.Text(c, "还有会话在运行，退出会关闭它们。").TextColor(t.TextMuted)
				ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
					if ui.Button(c, "取消").Clicked() {
						a.showQuit = false
					}
					if ui.PrimaryButton(c, "退出").Clicked() {
						a.showQuit = false
						a.quit()
					}
				})
			})
		})
	}
	if a.showGuardAsk {
		m := ui.Modal(c, &a.showGuardAsk, func() {
			req, rule := a.guardReq, a.guardRule
			ui.Column(c.Key("guard-dialog")).Width(560).Gap(12).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, icoShield)
					ui.Text(c, "AI Agent 请求执行受控命令").FontSize(16).Bold()
				})
				if req != nil {
					ui.Text(c, fmt.Sprintf("工具 %s · 客户端 %s · 会话 %s", req.Tool, orDash(req.Client), orDash(req.Session))).TextColor(t.TextMuted).FontSize(12)
				}
				ui.Box(c).Padding(10).Radius(6).Background(t.Surface).Children(func() {
					ui.Text(c, orDash(req.Command)).Wrap()
				})
				if rule != nil {
					ui.Text(c, "命中规则："+rule.Reason+"（"+rule.Source+"）").TextColor(t.TextMuted).FontSize(12)
				}
				ui.Text(c, "拒绝会返回错误给 Agent；允许只放行这一条命令。").FontSize(12).TextColor(t.TextMuted)
				ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
					if ui.Button(c, "拒绝").Clicked() {
						a.answerGuard(false)
					}
					if ui.PrimaryButton(c, "允许本次").Clicked() {
						a.answerGuard(true)
					}
				})
			})
		})
		if m.Dismissed() {
			a.answerGuard(false) // Escape 或点击外部：当作拒绝
		}
	}
	if a.showHostKey {
		m := ui.Modal(c, &a.showHostKey, func() {
			ui.Column(c.Key("hostkey-dialog")).Width(460).Gap(12).Children(func() {
				ui.Text(c, "首次连接 "+a.hkHost).FontSize(16).Bold()
				ui.Text(c, "无法确认主机身份，请核对指纹：").TextColor(t.TextMuted)
				ui.Box(c).Padding(10).Radius(6).Background(t.Surface).Children(func() {
					ui.Column(c).Gap(4).Children(func() {
						ui.Text(c, "SHA256:"+a.hkFP).FontSize(12)
						ui.Text(c, "主机身份以此指纹为准。").FontSize(12).TextColor(t.TextMuted)
					})
				})
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, icoAlert).TextColor(t.Danger).FontSize(14)
					ui.Text(c, "以后该主机指纹若变化，连接将被拒绝。").FontSize(12).TextColor(t.Danger)
				})
				ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
					if ui.Button(c, "拒绝").Clicked() {
						a.answerHostKey(false)
					}
					if ui.PrimaryButton(c, "信任并保存").Clicked() {
						a.answerHostKey(true)
					}
				})
			})
		})
		if m.Dismissed() {
			a.answerHostKey(false) // Escape 或点击外部：当作拒绝
		}
	}
	if a.showSSH {
		ui.Modal(c, &a.showSSH, func() {
			t := c.Theme()
			ui.Column(c.Key("ssh-dialog")).Width(460).Gap(12).Children(func() {
				ui.Text(c, "SSH 连接").FontSize(16).Bold()
				if len(a.store.Profiles) > 0 {
					ui.MenuButton(c, "选择已存档案…", func(m *ui.Menu) {
						for _, p := range a.store.Profiles {
							if m.Item(p.Name).Chosen() {
								a.fillSSH(p)
							}
						}
					})
				}
				ui.Row(c).Gap(8).Children(func() {
					ui.TextInput(c.Key("ssh-host"), &a.sshHost).Placeholder("主机").Grow(1)
					ui.TextInput(c.Key("ssh-port"), &a.sshPort).Placeholder("端口").Width(90)
				})
				ui.TextInput(c.Key("ssh-user"), &a.sshUser).Placeholder("用户名")
				ui.MenuButton(c, "认证方式："+authLabel(a.sshAuth), func(m *ui.Menu) {
					for _, k := range []string{"password", "key", "agent"} {
						if m.Item(authLabel(k)).Chosen() {
							a.sshAuth = k
						}
					}
				})
				switch a.sshAuth {
				case "key":
					ui.TextInput(c.Key("ssh-keypath"), &a.sshKeyPath).Placeholder(`私钥路径，如 C:\Users\you\.ssh\id_ed25519`)
					ui.TextInput(c.Key("ssh-passphrase"), &a.sshPass).Placeholder("私钥口令（可空）").Password()
				case "agent":
					ui.Text(c, "使用系统 OpenSSH Agent（服务需在运行）。").TextColor(t.TextMuted).FontSize(12)
				default:
					ui.TextInput(c.Key("ssh-password"), &a.sshPass).Placeholder("密码").Password()
				}
				ui.Text(c, "首次连接会请求确认主机指纹。").TextColor(t.TextMuted).FontSize(12)
				ui.Checkbox(c, &a.sshSave, "保存为档案")
				if a.sshSave {
					ui.TextInput(c.Key("ssh-saveas"), &a.sshSaveAs).Placeholder("档案名称")
				}
				ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
					if ui.Button(c, "取消").Clicked() {
						a.showSSH = false
					}
					if ui.PrimaryButton(c, "连接").Clicked() {
						a.connectSSH()
					}
				})
			})
		})
	}
	if a.showCustom {
		ui.Modal(c, &a.showCustom, func() {
			ui.Column(c.Key("custom-dialog")).Width(440).Gap(12).Children(func() {
				ui.Text(c, "自定义命令").FontSize(16).Bold()
				ui.Text(c, "要在终端里运行的命令行，例如 ssh user@host").TextColor(t.TextMuted)
				if ui.TextInput(c.Key("custom-cmd"), &a.customCmd).Placeholder("命令行").AutoFocus().Submitted() {
					a.startCustom()
				}
				ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
					if ui.Button(c, "取消").Clicked() {
						a.showCustom = false
					}
					if ui.PrimaryButton(c, "打开").Clicked() {
						a.startCustom()
					}
				})
			})
		})
	}
}

// newSessionMenu is the 新建 button's menu, shortcuts annotated.
func (a *app) newSessionMenu(m *ui.Menu) {
	if m.Item("新建终端").Shortcut(ui.Cmd|ui.Shift, ui.KeyT).Chosen() {
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
func (a *app) sessionMenu(m *ui.Menu, s *engine.Session) {
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
	if s.Kind() == engine.SSH && s.State() == engine.Exited {
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
}

// stateMark is the little dot that tells a session's state, in the
// theme's own colors; the bot stands for a session an AI agent operates.
func (a *app) stateMark(c *ui.Context, s *engine.Session) {
	if on, _ := s.AgentLock(); on {
		ui.Icon(c, icoBot).FontSize(13)
		return
	}
	t := c.Theme()
	color := t.Success
	switch s.State() {
	case engine.Exited:
		color = t.TextMuted
	case engine.Starting:
		color = t.Warning
	}
	ui.Icon(c, icoDot).FontSize(9).TextColor(color)
}

// findBar is the terminal search bar: text, counter, prev and next.
func (a *app) findBar(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Padding(6, 8).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, icoSearch).TextColor(t.TextMuted)
		in := ui.TextInput(c.Key("find-input"), &a.findText).Placeholder("搜索终端").Width(240)
		if in.Changed() {
			a.searchFor(a.findText)
		}
		if in.Submitted() {
			a.searchNext()
		}
		ui.Text(c, a.findStatus()).TextColor(t.TextMuted).FontSize(12).Grow(1)
		if ui.Button(c, "").Label("上一个匹配").Padding(3, 6).Children(func() {
			ui.Icon(c, icoChevronUp)
		}).Clicked() {
			a.searchPrev()
		}
		if ui.Button(c, "").Label("下一个匹配").Padding(3, 6).Children(func() {
			ui.Icon(c, icoChevronDown)
		}).Clicked() {
			a.searchNext()
		}
		if ui.Button(c, "").Label("关闭搜索").Padding(3, 6).Children(func() {
			ui.Icon(c, icoX)
		}).Clicked() {
			a.closeFind()
		}
	})
}

// tabBar is the strip of session tabs, with a quick 新建 at its end.
func (a *app) tabBar(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Background(t.Surface).Padding(4, 6).Gap(2).AlignItems(ui.Center).Children(func() {
		sessions := a.mgr.List()
		if len(sessions) == 0 {
			ui.Text(c, "无会话").TextColor(t.TextMuted)
		}
		for _, s := range sessions {
			a.tab(c, s)
		}
		ui.Spacer(c)
		ui.Button(c, "").Label("新建会话").Tooltip("Ctrl+Shift+T").Padding(4, 7).
			Children(func() { ui.Icon(c, icoPlus).FontSize(14) }).Menu(a.newSessionMenu)
	})
}

// tab is one session's tab: a title button beside a close button that
// shows itself when the pointer is over the tab; the session's menu on a
// right click.
func (a *app) tab(c *ui.Context, s *engine.Session) {
	t := c.Theme()
	active := s.ID() == a.activeID
	ui.Row(c.Key(s.ID())).Gap(1).AlignItems(ui.Center).Children(func() {
		var hovered bool
		bg := ui.Color{}
		if active {
			bg = t.Background
		}
		b := ui.Button(c, "").Padding(6, 10).Radius(6).Background(bg).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				a.stateMark(c, s)
				ui.Text(c, s.Title()).MaxWidth(170).Ellipsis("…")
			})
		})
		hovered = b.Hovered()
		if b.Clicked() {
			a.viewSession(s.ID())
		}
		visible := active || hovered
		if ui.Button(c, "").Label("关闭会话").Padding(3, 5).Children(func() {
			x := ui.Icon(c, icoX).FontSize(13)
			if !visible {
				x = x.TextColor(ui.RGBA(0, 0, 0, 0))
			}
		}).Clicked() {
			a.closeSession(s)
		}
	}).ContextMenu(func(m *ui.Menu) { a.sessionMenu(m, s) })
}

// buildPaneTree renders the split tree, leaves showing their sessions.
func (a *app) buildPaneTree(c *ui.Context, n *paneNode) {
	t := c.Theme()
	if n == nil {
		ui.Column(c).Fill().Center().Gap(12).Children(func() {
			ui.Icon(c, icoTerminal).FontSize(30).TextColor(t.TextMuted)
			ui.Text(c, "从标签栏的「新建」开始")
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				kbd(c, "Ctrl+Shift+T")
				ui.Text(c, "新建终端").TextColor(t.TextMuted).FontSize(12)
				ui.Text(c, "·").TextColor(t.TextMuted).FontSize(12)
				kbd(c, "Ctrl+F")
				ui.Text(c, "搜索").TextColor(t.TextMuted).FontSize(12)
			})
			ui.Text(c, "右键标签或终端可分屏").TextColor(t.TextMuted).FontSize(12)
		})
		return
	}
	if n.split != nil {
		ui.Split(c, &n.split.size,
			func() { a.buildPaneTree(c, n.split.first) },
			func() { a.buildPaneTree(c, n.split.second) }).Fill()
		return
	}
	a.paneView(c, n)
}

// paneView is one leaf: the session's terminal with an accent border
// while focused, an operation mask while an agent works it, and the
// pane's menu on a right click.
func (a *app) paneView(c *ui.Context, n *paneNode) {
	t := c.Theme()
	var session *engine.Session
	if n.sessionID != "" {
		session, _ = a.mgr.Get(n.sessionID)
	}
	focused := n == a.focus && n.sessionID != ""
	box := ui.Box(c).Fill()
	if focused {
		box = box.Border(1, t.Accent)
	}
	box.Children(func() {
		if n.sessionID == "" {
			ui.Column(c).Fill().Center().Gap(8).Children(func() {
				ui.Spinner(c)
				ui.Text(c, "正在启动…").TextColor(t.TextMuted)
			})
			return
		}
		if session == nil || session.Terminal() == nil {
			ui.Column(c).Fill().Center().Children(func() {
				ui.Text(c, "会话已关闭").TextColor(t.TextMuted)
			})
			return
		}
		v := terminal.View(c, session.Terminal()).Fill()
		if focused {
			v = v.AutoFocus()
		}
		if on, tool := session.AgentLock(); on {
			a.agentMask(c, tool)
		}
	})
	if box.Clicked() {
		a.focus = n
		a.activeID = n.sessionID
	}
	box.ContextMenu(func(m *ui.Menu) { a.paneMenu(m, n) })
}

// agentMask covers a pane an AI agent is operating: a translucent veil
// with a card, so the human watches live while nothing typed reaches the
// session. AutoFocus pulls the focus away from the terminal the moment
// the mask appears.
func (a *app) agentMask(c *ui.Context, tool string) {
	t := c.Theme()
	ui.Box(c).Absolute().Top(0).Bottom(0).Left(0).Right(0).
		Background(colorMask).AutoFocus().
		Children(func() {
			ui.Column(c).Fill().Center().Children(func() {
				ui.Box(c).Width(380).Padding(20, 24).Radius(10).
					Background(t.Surface).Border(1, t.Border).Children(func() {
					ui.Column(c).Center().Gap(12).Children(func() {
						ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
							ui.Icon(c, icoBot).FontSize(22).TextColor(t.Accent)
							ui.Text(c, "AI Agent 操作中").Bold().FontSize(16)
						})
						ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
							ui.Spinner(c)
							ui.Text(c, "正在运行 "+tool).FontSize(12)
						})
						ui.Text(c, "输入已锁定，结束后自动恢复").FontSize(12).TextColor(t.TextMuted)
					})
				})
			})
		})
}

// paneMenu is a pane's right-click menu. A right click focuses the pane
// it landed on, so the actions apply to what the user pointed at;
// destructive items refuse while an agent operates their session.
func (a *app) paneMenu(m *ui.Menu, n *paneNode) {
	if n.sessionID != "" {
		a.focus = n
		a.activeID = n.sessionID
	}
	if m.Item("向右分屏").Chosen() {
		a.splitFocused(true)
	}
	if m.Item("向下分屏").Chosen() {
		a.splitFocused(false)
	}
	m.Separator()
	label := "关闭分格"
	disabled := false
	if n.sessionID != "" {
		if s, ok := a.mgr.Get(n.sessionID); ok {
			if on, _ := s.AgentLock(); on {
				label = "关闭分格（Agent 操作中）"
				disabled = true
			}
		}
	}
	if m.Item(label).Disabled(disabled).Chosen() {
		a.closePane(n, false)
	}
}

// terminalArea shows the split panes of the terminal area.
func (a *app) terminalArea(c *ui.Context) {
	a.buildPaneTree(c, a.splitRoot)
}

// auditView is the audit window: its own title strip over the entries,
// refreshed every couple of seconds while the window shows.
func (a *app) auditView(c *ui.Context) {
	t := c.Theme()
	var entries []audit.Entry
	total := int64(0)
	if a.auditLog != nil {
		entries, _ = a.auditLog.Tail(200)
		total = a.auditLog.Written()
	}
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		titleStrip(c, "MCP 审计日志", func() {
			ui.Text(c, fmt.Sprintf("累计 %d 条 · 显示最近 %d 条", total, len(entries))).TextColor(t.TextMuted).FontSize(12)
		})
		ui.Divider(c)
		if len(entries) == 0 {
			ui.Column(c).Fill().Center().Gap(8).Children(func() {
				ui.Icon(c, icoHistory).FontSize(28).TextColor(t.TextMuted)
				ui.Text(c, "还没有 MCP 调用记录").TextColor(t.TextMuted)
			})
		} else {
			ui.Scroll(c).Fill().Children(func() {
				ui.Column(c).Gap(2).Padding(4, 8).Children(func() {
					for i := len(entries) - 1; i >= 0; i-- { // newest first
						a.auditRow(c, entries[i])
					}
				})
			})
		}
	})
	// The window repaints with fresh entries on its own.
	c.After(2 * time.Second)
}

// auditRow is one call: time, outcome, tool, command and detail.
func (a *app) auditRow(c *ui.Context, e audit.Entry) {
	t := c.Theme()
	when := e.Time.Format("01-02 15:04:05")
	ico, color := icoCheck, t.Success
	switch {
	case e.Guard != "" && strings.HasPrefix(e.Guard, "denied"):
		ico, color = icoAlert, t.Warning
	case !e.OK:
		ico, color = icoCircleX, t.Danger
	case e.Guard == "confirmed":
		ico, color = icoCheck, t.Warning
	}
	ui.Row(c.Key(fmt.Sprintf("audit-%d-%s", e.Time.UnixNano(), e.Tool))).
		Gap(8).Padding(4, 6).Radius(4).AlignItems(ui.Center).
		Children(func() {
			ui.Text(c, when).TextColor(t.TextMuted).FontSize(11)
			ui.Icon(c, ico).FontSize(13).TextColor(color)
			ui.Text(c, e.Tool).Bold().FontSize(12)
			cmd := e.Command
			if cmd == "" {
				cmd = e.Tool
			}
			ui.Text(c, cmd).MaxWidth(360).Ellipsis("…").FontSize(12).Grow(1)
			detail := e.Client
			if e.Err != "" {
				detail = strings.Join([]string{e.Client, "错误: " + firstLine(e.Err)}, " · ")
			}
			ui.Text(c, detail).TextColor(t.TextMuted).FontSize(11).MaxWidth(180).Ellipsis("…")
		}).ContextMenu(func(m *ui.Menu) {
		full := e.Command
		if full != "" {
			if m.Item("复制命令").Chosen() {
				mygo.Clipboard.WriteText(full)
			}
		}
		if e.Err != "" {
			if m.Item("复制错误").Chosen() {
				mygo.Clipboard.WriteText(e.Err)
			}
		}
	})
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// popoutView is a synchronized popout window: its own title strip over
// the session's mirror terminal.
func (a *app) popoutView(c *ui.Context, mv *engine.MirrorView, title string) {
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		titleStrip(c, title+" — 同屏", nil)
		ui.Divider(c)
		if term := mv.Terminal(); term != nil {
			terminal.View(c, term).Fill().AutoFocus()
			return
		}
		ui.Column(c).Fill().Center().Children(func() {
			ui.Text(c, "会话已结束").TextColor(t.TextMuted)
		})
	})
}

// ---- helpers ----

func (a *app) active() *engine.Session {
	s, _ := a.mgr.Get(a.activeID)
	return s
}

func (a *app) sessionsOf(kind engine.Kind) []*engine.Session {
	var out []*engine.Session
	for _, s := range a.mgr.List() {
		if s.Kind() == kind {
			out = append(out, s)
		}
	}
	return out
}
