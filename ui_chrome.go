package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// icoShield is the guard dialog's icon: a shield with a check.
var icoShield = iconSVG(`<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/><path d="m9 12 2 2 4-4"/>`)

// icoShieldX is a denied call in the audit list.
var icoShieldX = iconSVG(`<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/><path d="m9.5 9.5 5 5"/><path d="m14.5 9.5-5 5"/>`)

// mainView builds the main window: a sidebar that runs to the top of the
// window beside the content pane — the title strip with the actions, the
// tab bar, the terminal panes and the status bar — with the dialogs over
// them.
func (a *app) mainView(c *ui.Context) {
	webTheme(c)
	c.SetMenuPresentation(ui.PopoverMenus)
	a.saveActive() // the tab bar and body show the active workspace
	if a.pendingToast != "" {
		c.Toast(a.pendingToast)
		a.pendingToast = ""
	}

	// ---- keyboard shortcuts (window level, ahead of the terminals) ----
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyT) {
		a.newSession(a.defaultShellID(), "", "")
	}
	if c.Shortcut(ui.Cmd, ui.KeyTab) {
		a.cycleTab(1)
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyTab) {
		a.cycleTab(-1)
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyW) {
		a.closeTab(a.activeTab)
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyF12) {
		if s := a.active(); s != nil {
			a.forceCloseSession(s)
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
	if c.Shortcut(ui.Cmd, ui.KeyB) {
		a.sidebarCollapsed = !a.sidebarCollapsed
	}

	ui.Column(c).Fill().Children(func() {
		a.appHeader(c)
		// The body alone collapses: the header and footer run across the
		// whole window, over the sidebar and the content alike.
		ui.Box(c).FillWidth().Grow(1).Children(func() {
			if a.sidebarCollapsed {
				ui.Row(c).Fill().Children(func() {
					a.sidebarRail(c)
					ui.Column(c).Grow(1).FillHeight().Children(func() { a.contentPane(c) })
				})
			} else {
				// The drag writes the width; keep it within what the rows
				// can still lay out in step.
				a.sidebarWidth = clampSidebarWidth(a.sidebarWidth)
				ui.Split(c, &a.sidebarWidth,
					func() { a.sidebarPane(c) },
					func() { a.contentPane(c) }).Fill()
			}
		})
		a.appFooter(c)
	})

	a.mainDialogs(c)
}

// sidebarPane is the left column below the shared header: the primary
// action, the session filter and the sessions, with the agent's live work
// at the foot.
func (a *app) sidebarPane(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Surface).
		BorderWidth(0, 1, 0, 0).BorderColor(t.Border).Children(func() {
		ui.Column(c).Padding(10, 10, 6, 10).Gap(8).Children(func() {
			hint := "Ctrl + Shift + T"
			if a.sidebarWidth < 230 {
				hint = "" // the chip would crowd the label
			}
			plainAction(c, "新建会话", hint, "新建会话（Ctrl+Shift+T）",
				icoPlus, a.newSessionMenu)
			a.sessionFilter(c)
		})
		a.sidebarSessions(c)
		if n, tool, target := a.agentSummary(); n > 0 {
			a.agentCard(c, n, tool, target)
		}
	})
}

// sidebarRail is the folded sidebar below the shared header: the
// new-session button, the filter and one chip per session. The header's
// brand button expands it again, and the footer carries MCP state.
func (a *app) sidebarRail(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Width(sidebarRailWidth).FillHeight().Shrink(0).
		Background(t.Surface).BorderWidth(0, 1, 0, 0).BorderColor(t.Border).
		Children(func() {
			// All spacing lives on the containers: the folded rail has no
			// per-item margins for the layout to lose.
			ui.Column(c).FillWidth().Padding(10, 0, 0, 0).Gap(8).
				AlignItems(ui.Center).Children(func() {
				a.railAction(c, icoPlus, "新建会话", "新建会话（Ctrl+Shift+T）",
					true, a.newSessionMenu, nil)
				a.railAction(c, icoSearch, "搜索会话", "展开侧栏并筛选会话",
					false, nil, func() { a.sidebarCollapsed = false })
			})

			ui.Column(c).FillWidth().Padding(14, 0).AlignItems(ui.Center).
				Children(func() {
					ui.Box(c).Width(28).Height(1).Background(t.Border).Shrink(0)
				})

			ui.Scroll(c).FillWidth().Grow(1).Children(func() {
				ui.Column(c).FillWidth().Padding(0, 0, 8, 0).Gap(6).
					AlignItems(ui.Center).Children(func() {
					for _, s := range a.mgr.List() {
						a.railSession(c, s)
					}
				})
			})
		})
}

// railAction is one folded-sidebar action: 36×36 on the rail's grid,
// filled for the primary action, a hover tint for every other one.
func (a *app) railAction(c *ui.Context, svg *ui.SVG, label, tip string,
	accent bool, build func(*ui.Menu), fn func()) {
	t := c.Theme()
	b := ui.Button(c, "").Label(label).Width(36).Height(36).Radius(10).
		Padding(0).Shrink(0).Background(ui.Transparent).BorderWidth(0)
	hovered, pressed := b.Hovered(), b.Pressed()
	color := t.TextMuted
	switch {
	case accent && pressed:
		b = b.Background(t.AccentPressed)
		color = t.AccentText
	case accent && hovered:
		b = b.Background(t.AccentHover)
		color = t.AccentText
	case accent:
		b = b.Background(t.Accent)
		color = t.AccentText
	case hovered:
		b = b.Background(t.SurfaceHover)
		color = t.Text
	}
	if build != nil {
		b = b.PopoverMenuButton(build)
	}
	b.Tooltip(tip).Children(func() {
		ui.Icon(c, svg).FontSize(20).TextColor(color)
	})
	if fn != nil && b.Clicked() {
		fn()
	}
}

// railSession is one session in the folded sidebar: a 36×36 chip with its
// type icon, a state dot in the lower right, and a persimmon edge when it
// is the active one.
func (a *app) railSession(c *ui.Context, s *engine.Session) {
	t := c.Theme()
	active := s.ID() == a.activeID
	icon := icoTerminal
	if s.Kind() == engine.SSH {
		icon = icoGlobe
	}

	state := t.Success
	switch s.State() {
	case engine.Exited:
		state = t.TextMuted
	case engine.Starting:
		state = t.Warning
	}
	on, _ := s.AgentLock()
	if on {
		state = t.Warning
	}

	b := ui.Button(c.Key("rail-"+s.ID()), "").Label(s.Title()).
		Width(36).Height(36).Radius(10).Padding(0).Shrink(0).
		Background(ui.Transparent).BorderWidth(0)
	hovered := b.Hovered()
	color := t.TextMuted
	switch {
	case active && hovered:
		b = b.Background(t.SurfacePressed)
		color = t.Text
	case active:
		b = b.Background(t.SurfaceHover)
		color = t.Text
	case hovered:
		b = b.Background(t.SurfaceHover.Alpha(0.6))
		color = t.Text
	case s.State() == engine.Exited:
		color = t.TextMuted.Alpha(0.65)
	}
	tip := s.Title()
	if s.State() == engine.Exited {
		tip += "（已退出）"
	}
	if on {
		tip += "（Agent 操作中）"
	}
	b.Tooltip(tip)
	b.PopoverMenu(func(m *ui.Menu) { a.sessionMenu(m, s) })
	b.Children(func() {
		ui.Icon(c, icon).FontSize(18).TextColor(color)
		if active {
			ui.Box(c).Width(2).Height(16).Radius(1).Background(t.Accent).
				Absolute().Left(4).Top(10).Shrink(0)
		}
		ui.Box(c).Width(6).Height(6).Radius(3).Background(state).
			Absolute().Right(4).Bottom(4).Shrink(0)
	})
	if b.Clicked() {
		a.viewSession(s.ID())
	}
}

// logoMark is the app's mark: a rounded accent tile with a terminal
// glyph punched into it.
func (a *app) logoMark(c *ui.Context) {
	t := c.Theme()
	ui.Box(c).Width(22).Height(22).Radius(6).Background(t.Accent).
		AlignItems(ui.Center).Justify(ui.Center).Shrink(0).Children(func() {
		ui.Icon(c, icoMark).FontSize(16).TextColor(t.AccentText)
	})
}

// sessionFilter is the sidebar's quiet search field; typing narrows the
// session lists below.
func (a *app) sessionFilter(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).FillWidth().Height(28).Padding(0, 8).Gap(6).Radius(6).
		Background(t.Background).Border(1, t.Border).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, icoSearch).FontSize(12).TextColor(t.TextMuted)
		ui.TextInput(c.Key("session-filter"), &a.filter).Grow(1).Height(24).
			FontSize(11.5).BorderWidth(0).Background(ui.Transparent).
			Placeholder("筛选会话…").PlaceholderColor(t.TextMuted)
		if a.filter != "" {
			if ui.Button(c, "").Label("清除筛选").Padding(1).Radius(4).
				Background(ui.Transparent).BorderWidth(0).Children(func() {
				ui.Icon(c, icoX).FontSize(11).TextColor(t.TextMuted)
			}).Clicked() {
				a.filter = ""
			}
		}
	})
}

// sidebarSessions is the session tree: local shells and SSH sessions,
// each row with its own icon and state.
func (a *app) sidebarSessions(c *ui.Context) {
	t := c.Theme()
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).FillWidth().Children(func() {
			local := a.sessionsOf(engine.Local)
			sectionHeader(c, "本地", len(local))
			ui.Column(c).FillWidth().Padding(0, 8, 6, 8).Gap(1).Children(func() {
				if len(local) == 0 && a.filter == "" {
					emptyNote(c, "尚无本地会话")
				}
				for _, s := range local {
					a.sessionRow(c, s)
				}
			})

			remote := a.sessionsOf(engine.SSH)
			sectionHeader(c, "SSH", len(remote))
			ui.Column(c).FillWidth().Padding(0, 8, 6, 8).Gap(1).Children(func() {
				if len(remote) == 0 && a.filter == "" {
					emptyNote(c, "尚无 SSH 会话")
				}
				for _, s := range remote {
					a.sessionRow(c, s)
				}
				add := ui.Row(c).FillWidth().Height(28).Padding(0, 10).Gap(7).
					Radius(6).AlignItems(ui.Center)
				if add.Hovered() {
					add = add.Background(t.SurfaceHover)
				}
				add.Children(func() {
					ui.Icon(c, icoPlus).FontSize(11).TextColor(t.Accent)
					ui.Text(c, "新建 SSH 连接").FontSize(11.5).TextColor(t.Accent)
				})
				if add.Clicked() {
					a.showSSH = true
				}
			})
		})
	})
}

// sessionRow is one session: the accent edge and a filled face while it is
// the active one, its icon, title and state.
func (a *app) sessionRow(c *ui.Context, s *engine.Session) {
	t := c.Theme()
	active := s.ID() == a.activeID
	exited := s.State() == engine.Exited

	row := ui.Row(c.Key(s.ID())).FillWidth().Height(30).Padding(0, 8).Gap(8).
		Radius(6).AlignItems(ui.Center)
	switch {
	case active:
		row = row.Background(t.SurfaceHover)
	case row.Hovered():
		row = row.Background(t.SurfaceHover.Alpha(0.55))
	}

	edge := ui.Transparent
	if active {
		edge = t.Accent
	}
	titleColor := t.Text
	iconColor := t.TextMuted
	if active {
		iconColor = t.Accent
	} else if exited {
		titleColor = t.TextMuted
	}

	row.Children(func() {
		ui.Box(c).Width(2).Height(14).Radius(1).Background(edge).Shrink(0)
		icon := icoTerminal
		if s.Kind() == engine.SSH {
			icon = icoGlobe
		}
		ui.Icon(c, icon).FontSize(13).TextColor(iconColor)
		ui.Text(c, s.Title()).FontSize(12).FontWeight(weightOf(active, 500, 400)).
			TextColor(titleColor).Grow(1).SingleLine().Ellipsis("…")
		a.stateMark(c, s)
	})
	row.Tooltip(s.Title())
	row.PopoverMenu(func(m *ui.Menu) { a.sessionMenu(m, s) })
	if row.Clicked() {
		a.viewSession(s.ID())
	}
}

// weightOf picks a font weight for an active or quiet row.
func weightOf(active bool, on, off int) int {
	if active {
		return on
	}
	return off
}

// agentCard is the live summary of the agent's work in the sidebar: how
// many sessions, which tool, which session.
func (a *app) agentCard(c *ui.Context, n int, tool, target string) {
	t := c.Theme()
	ui.Column(c).FillWidth().Padding(0, 10, 8, 10).Children(func() {
		ui.Column(c).FillWidth().Padding(10).Gap(7).Radius(10).
			Background(t.Surface).Border(1, t.Warning.Alpha(0.28)).Children(func() {
			ui.Row(c).FillWidth().Gap(7).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, icoBot).FontSize(13).TextColor(t.Warning)
				ui.Text(c, "AI Agent 操作中").FontSize(11).FontWeight(500).TextColor(t.Warning)
				ui.Spacer(c)
				chip(c, fmt.Sprint(n), t.Warning, t.Warning.Alpha(0.14))
			})
			line := tool
			if target != "" {
				line += " · " + target
			}
			if line == "" {
				line = "等待工具调用"
			}
			mono(c, line).FontSize(9.5).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "输入已锁定").FontSize(9.5).TextColor(t.TextMuted)
				ui.Spacer(c)
				ui.Text(c, "详情见分格").FontSize(9.5).TextColor(t.TextMuted)
			})
		})
	})
}

// appFooter is the one status strip along the bottom of the whole window:
// the MCP endpoint and its port at left, the session count and the active
// session in the middle, the agent and stdio at right. It stays whole as
// the sidebar folds.
func (a *app) appFooter(c *ui.Context) {
	t := c.Theme()
	port := a.store.Settings.MCPHTTPPort
	if port == 0 {
		port = 8731
	}
	ui.Row(c).FillWidth().Height(statusBarHeight).Padding(0, 12).
		Background(t.Surface).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).
		AlignItems(ui.Center).Gap(9).Children(func() {
		if a.mcpUp {
			dot(c, t.Success, 5)
			ui.Text(c, "MCP 已就绪").FontSize(10.5).TextColor(t.TextMuted)
		} else {
			dot(c, t.TextMuted, 5)
			ui.Text(c, "MCP 未启动").FontSize(10.5).TextColor(t.TextMuted)
		}
		mono(c, fmt.Sprintf(":%d", port)).FontSize(9.5).TextColor(t.TextMuted)
		if ui.Button(c, "").Label("复制端口").Padding(2).Radius(4).
			Background(ui.Transparent).BorderWidth(0).Tooltip("复制 MCP 端口").Children(func() {
			ui.Icon(c, icoCopy).FontSize(11).TextColor(t.TextMuted)
		}).Clicked() {
			mygo.Clipboard.WriteText(fmt.Sprint(port))
			a.pendingToast = "MCP 端口已复制"
			a.refreshUI()
		}
		verticalRule(c)

		n := len(a.mgr.List())
		color := t.Success
		if n == 0 {
			color = t.TextMuted
		}
		dot(c, color, 5)
		ui.Text(c, fmt.Sprintf("%d 个会话", n)).FontSize(10.5).TextColor(t.TextMuted)
		if s := a.active(); s != nil {
			ui.Text(c, "·").FontSize(10.5).TextColor(t.TextMuted.Alpha(0.5))
			ui.Text(c, s.Title()).FontSize(10.5).TextColor(t.TextMuted).
				MaxWidth(220).SingleLine().Ellipsis("…")
		}
		ui.Spacer(c)
		if agents := a.agentBusy(); agents > 0 {
			ui.Icon(c, icoBot).FontSize(12).TextColor(t.Warning)
			ui.Text(c, fmt.Sprintf("Agent ×%d", agents)).FontSize(10.5).
				FontWeight(500).TextColor(t.Warning)
			verticalRule(c)
		}
		ui.Text(c, "stdio 可用").FontSize(10.5).TextColor(t.TextMuted)
	})
}

// contentPane is the right column below the shared header: the tab bar
// and the terminal panes.
func (a *app) contentPane(c *ui.Context) {
	ui.Column(c).Fill().Background(c.Theme().Background).Children(func() {
		a.tabBar(c)
		a.terminalArea(c)
	})
}

// appHeader is the window's one title strip: the brand and the rail's
// collapse button at left, the active session's context in the middle,
// the agent's presence and the window actions at right. It runs across
// the sidebar and the content, and stays whole as the sidebar folds.
func (a *app) appHeader(c *ui.Context) {
	bar := c.TitleBar()
	t := c.Theme()
	ui.Row(c).FillWidth().Height(chromeBarHeight(c)).
		Padding(0, bar.Right+10, 0, bar.Left+14).
		Background(t.Surface).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).
		DragWindow().AlignItems(ui.Center).Gap(9).Children(func() {
		a.logoMark(c)
		ui.Text(c, "AdaKaki").FontSize(13.5).FontWeight(600)
		chip(c, "v0.2", t.TextMuted, t.Background)
		if a.sidebarCollapsed {
			ghostButton(c, icoPanelLeft, "展开侧栏", "展开侧栏（Ctrl+B）",
				func() { a.sidebarCollapsed = false })
		} else {
			ghostButton(c, icoPanelLeft, "收起侧栏", "收起侧栏（Ctrl+B）",
				func() { a.sidebarCollapsed = true })
		}
		verticalRule(c)

		if s := a.active(); s != nil {
			a.stateMark(c, s)
			ui.Text(c, s.Title()).FontSize(12.5).FontWeight(500).
				MaxWidth(280).SingleLine().Ellipsis("…")
			kind := "本地"
			if s.Kind() == engine.SSH {
				kind = "SSH"
			}
			chip(c, kind, t.TextMuted, t.Background)
		} else {
			ui.Text(c, "没有活动会话").FontSize(12).TextColor(t.TextMuted)
		}
		ui.Spacer(c)
		if n, _, _ := a.agentSummary(); n > 0 {
			ui.Row(c).Padding(3, 8).Gap(5).Radius(6).
				Background(t.Warning.Alpha(0.14)).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, icoBot).FontSize(12).TextColor(t.Warning)
				ui.Text(c, fmt.Sprintf("Agent ×%d", n)).FontSize(10.5).
					FontWeight(500).TextColor(t.Warning)
			})
			verticalRule(c)
		}
		ghostButton(c, icoHistory, "审计日志", "查看 MCP 调用审计", a.openAudit)
		ghostButton(c, icoSettings, "设置", "打开设置", a.openSettings)
		dark := t.Dark
		themeIcon, themeTip := icoMoon, "切换到深色"
		if dark {
			themeIcon, themeTip = icoSun, "切换到浅色"
		}
		ghostButton(c, themeIcon, "切换明暗", themeTip, func() {
			if dark {
				a.setAppearance("light")
			} else {
				a.setAppearance("dark")
			}
		})
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
				dialogTitle(c, icoAlert, "退出 AdaKaki？", true)
				if agents > 0 {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Icon(c, icoBot).FontSize(14).TextColor(t.Danger)
						ui.Text(c, fmt.Sprintf("AI Agent 正在操作 %d 个会话，退出会中断它们", agents)).
							FontSize(12).TextColor(t.Danger)
					})
				}
				ui.Text(c, "还有会话在运行，退出会关闭它们。").FontSize(12.5).TextColor(t.TextMuted)
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
				dialogTitle(c, icoShield, "AI Agent 请求执行受控命令", false)
				if req != nil {
					ui.Text(c, fmt.Sprintf("工具 %s · 客户端 %s · 会话 %s",
						req.Tool, orDash(req.Client), orDash(req.Session))).
						FontSize(11.5).TextColor(t.TextMuted)
				}
				codeBlock(c, func() {
					mono(c, orDash(req.Command)).FontSize(11.5).Wrap()
				})
				if rule != nil {
					ui.Text(c, "命中规则："+rule.Reason+"（"+rule.Source+"）").
						FontSize(11.5).TextColor(t.TextMuted)
				}
				ui.Text(c, "拒绝会返回错误给 Agent；允许只放行这一条命令。").
					FontSize(11.5).TextColor(t.TextMuted)
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
				dialogTitle(c, icoShield, "首次连接 "+a.hkHost, false)
				ui.Text(c, "无法确认主机身份，请核对指纹：").FontSize(12).TextColor(t.TextMuted)
				codeBlock(c, func() {
					ui.Column(c).Gap(5).Children(func() {
						mono(c, "SHA256:"+a.hkFP).FontSize(11).Wrap()
						ui.Text(c, "主机身份以此指纹为准。").FontSize(11).TextColor(t.TextMuted)
					})
				})
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, icoAlert).FontSize(13).TextColor(t.Danger)
					ui.Text(c, "以后该主机指纹若变化，连接将被拒绝。").
						FontSize(11.5).TextColor(t.Danger)
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
			ui.Column(c.Key("ssh-dialog")).Width(460).Gap(12).Children(func() {
				dialogTitle(c, icoGlobe, "SSH 连接", false)
				if len(a.store.Profiles) > 0 {
					ui.PopoverMenuButton(c, "选择已存档案…", func(m *ui.Menu) {
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
				ui.PopoverMenuButton(c, "认证方式："+authLabel(a.sshAuth), func(m *ui.Menu) {
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
					ui.Text(c, "使用系统 OpenSSH Agent（服务需在运行）。").
						FontSize(11.5).TextColor(t.TextMuted)
				default:
					ui.TextInput(c.Key("ssh-password"), &a.sshPass).Placeholder("密码").Password()
				}
				ui.Text(c, "首次连接会请求确认主机指纹。").
					FontSize(11.5).TextColor(t.TextMuted)
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
				dialogTitle(c, icoTerminal, "自定义命令", false)
				ui.Text(c, "要在终端里运行的命令行，例如 ssh user@host").
					FontSize(12.5).TextColor(t.TextMuted)
				if ui.TextInput(c.Key("custom-cmd"), &a.customCmd).Placeholder("命令行").
					AutoFocus().Submitted() {
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
			Shortcut(ui.Cmd|ui.Shift, ui.KeyF12).Chosen() {
			a.forceCloseSession(s)
		}
	}
}

// stateMark is the little dot that tells a session's state, in the
// theme's own colors; the bot stands for a session an AI agent operates.
func (a *app) stateMark(c *ui.Context, s *engine.Session) {
	t := c.Theme()
	if on, _ := s.AgentLock(); on {
		ui.Icon(c, icoBot).FontSize(12).TextColor(t.Warning)
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

// terminalArea shows the split panes as cards over the window's paper,
// with the find overlay above them when it is open.
func (a *app) terminalArea(c *ui.Context) {
	t := c.Theme()
	ui.Box(c).Fill().Background(t.Background).Children(func() {
		ui.Box(c).Fill().Padding(paneGap, paneGap, paneGap, paneGap).Children(func() {
			a.paneTree(c, a.splitRoot)
		})
		if a.findOpen {
			a.findOverlay(c)
		}
	})
}

// paneTree lays out a node of the split tree: a pane's card, or two
// subtrees with a divider between them that drags for the share of room.
func (a *app) paneTree(c *ui.Context, n *paneNode) {
	if n == nil {
		a.emptyPanes(c)
		return
	}
	if a.zoom != nil {
		// A zoomed pane takes the whole area, whatever its split.
		a.paneCard(c, a.zoom).Fill()
		return
	}
	if n.split == nil {
		a.paneCard(c, n).Fill()
		return
	}
	split := n.split
	var box ui.Element
	if split.vertical {
		// side by side: first at left, second at right
		box = ui.Row(c.Key(spKey(n)))
	} else {
		// one above the other
		box = ui.Column(c.Key(spKey(n)))
	}
	box.Fill().MinWidth(0).MinHeight(0).AlignItems(ui.Stretch)
	bounds := box.Bounds()
	first, second, ratio := split.first, split.second, split.ratio
	box.Children(func() {
		ui.Box(c).Grow(ratio).Basis(0).MinWidth(0).MinHeight(0).Children(func() {
			a.paneTree(c, first)
		})
		a.paneDivider(c, split, bounds)
		ui.Box(c).Grow(1 - ratio).Basis(0).MinWidth(0).MinHeight(0).Children(func() {
			a.paneTree(c, second)
		})
	})
}

// spKey names a split's layout node, so its bounds survive a frame.
func spKey(n *paneNode) string {
	if n.split == nil {
		return "pane-none"
	}
	return fmt.Sprintf("split-%s-%s-%v", n.split.first.sessionID, n.split.second.sessionID, n.split.vertical)
}

// paneDivider is the gap between two panes: it drags to move the line
// between them, and a double click splits the room evenly again.
func (a *app) paneDivider(c *ui.Context, split *splitNode, bounds ui.Rect) {
	t := c.Theme()
	div := ui.Box(c).Role(ui.RoleSplitter).Label("分格分隔线").Shrink(0)
	if split.vertical {
		div = div.Width(paneGap).Cursor(ui.CursorResizeColumn)
	} else {
		div = div.Height(paneGap).Cursor(ui.CursorResizeRow)
	}
	if dx, dy, ok := div.Dragged(); ok {
		total, d := bounds.W, dx
		if !split.vertical {
			total, d = bounds.H, dy
		}
		if total > 0 {
			split.ratio = min(max(split.ratio+d/total, 0.08), 0.92)
		}
	}
	if div.DoubleClicked() {
		split.ratio = 0.5
	}
	if div.Hovered() || div.Dragging() || div.Pressed() {
		div.Draw(func(p *ui.Painter, r ui.Rect) {
			if split.vertical {
				p.Fill(ui.Rect{X: r.X + r.W/2 - 1.5, Y: r.Y + r.H/2 - 18, W: 3, H: 36}, t.TextMuted.Alpha(0.55), 1.5)
			} else {
				p.Fill(ui.Rect{X: r.X + r.W/2 - 18, Y: r.Y + r.H/2 - 1.5, W: 36, H: 3}, t.TextMuted.Alpha(0.55), 1.5)
			}
		})
	}
}

// emptyPanes is the area with no session at all: the mark and the way to
// start one.
func (a *app) emptyPanes(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Gap(12).Children(func() {
		ui.Icon(c, icoTerminal).FontSize(28).TextColor(t.TextMuted)
		ui.Text(c, "从左上角「新建会话」开始").FontSize(13).TextColor(t.Text)
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			kbd(c, "Ctrl + Shift + T")
			ui.Text(c, "新建终端").FontSize(11.5).TextColor(t.TextMuted)
			ui.Text(c, "·").FontSize(11.5).TextColor(t.TextMuted)
			kbd(c, "Ctrl + F")
			ui.Text(c, "搜索").FontSize(11.5).TextColor(t.TextMuted)
		})
		ui.Text(c, "右键标签或终端可分屏").FontSize(11.5).TextColor(t.TextMuted)
	})
}

// findOverlay is the terminal search: a floating card in the pane's top
// right, so the terminal keeps its height.
func (a *app) findOverlay(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Absolute().Top(10).Right(24).Width(420).Height(40).Padding(0, 8).
		Gap(8).Radius(8).Background(t.Surface).Border(1, t.Border).
		Shadow(0, 12, 28, -6, ui.RGBA(0, 0, 0, 0.28)).
		AlignItems(ui.Center).Children(func() {
		ui.Icon(c, icoSearch).FontSize(13).TextColor(t.TextMuted)
		ui.TextInput(c.Key("find-input"), &a.findText).Grow(1).Height(28).
			FontSize(12).BorderWidth(0).Background(ui.Transparent).
			Placeholder("搜索终端").PlaceholderColor(t.TextMuted)
		if a.findText != "" {
			ui.Text(c, a.findStatus()).Font(monoFamily()).FontSize(10.5).
				TextColor(t.TextMuted)
		}
		buttons := []struct {
			icon       *ui.SVG
			label, tip string
			fn         func()
		}{
			{icoChevronUp, "上一个匹配", "上一个（Shift+Enter）", a.searchPrev},
			{icoChevronDown, "下一个匹配", "下一个（Enter）", a.searchNext},
			{icoX, "关闭搜索", "关闭（Esc）", a.closeFind},
		}
		for _, b := range buttons {
			btn := ui.Button(c, "").Label(b.label).Padding(3).Radius(5).
				Background(ui.Transparent).BorderWidth(0).Shrink(0)
			if btn.Hovered() {
				btn = btn.Background(t.SurfaceHover)
			}
			btn.Tooltip(b.tip).Children(func() {
				ui.Icon(c, b.icon).FontSize(12).TextColor(t.TextMuted)
			})
			if btn.Clicked() {
				b.fn()
			}
		}
	})
}

// tabBar is the strip of workspace tabs: several terminals in split
// panes make one tab, and a quick 新建 at its end.
func (a *app) tabBar(c *ui.Context) {
	t := c.Theme()
	a.saveActive()
	ui.Row(c).FillWidth().Height(tabBarHeight).Background(t.Surface).
		Padding(0, 8).Gap(2).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		if len(a.tabs) == 0 {
			ui.Text(c, "无标签页").FontSize(11.5).TextColor(t.TextMuted)
		}
		for _, ws := range a.tabs {
			a.workspaceTab(c, ws)
		}
		ui.Spacer(c)
		add := ui.Button(c, "").Label("新建会话").Tooltip("Ctrl+Shift+T").
			Padding(4).Radius(6).Background(ui.Transparent).BorderWidth(0).Shrink(0)
		if add.Hovered() {
			add = add.Background(t.SurfaceHover)
		}
		add.PopoverMenuButton(a.newSessionMenu).Children(func() {
			ui.Icon(c, icoPlus).FontSize(13).TextColor(t.TextMuted)
		})
		ghostButton(c, icoLayout, "向下分屏", "向下分屏", func() { a.splitFocused(false) })
		ghostButton(c, icoSearch, "终端搜索", "搜索终端（Ctrl+F）", a.openFind)
	})
}

// workspaceTab is one tab: a session's short title, its state dot and,
// when the tab holds several panes, their count; the close button closes
// every session in the tab.
func (a *app) workspaceTab(c *ui.Context, ws *workspace) {
	t := c.Theme()
	active := ws == a.activeTab
	s := a.representativeSession(ws)
	title := "新标签页"
	if s != nil {
		title = shortSessionTitle(s)
	}
	pill := ui.Row(c.Key(fmt.Sprintf("tab-%d", ws.id))).Height(tabPillHeight).
		Padding(0, 4, 0, 6).Gap(1).Radius(7, 7, 0, 0).
		AlignItems(ui.Center).AlignSelf(ui.End)
	switch {
	case active:
		pill = pill.Background(t.Background).Border(1, t.Border)
	case pill.Hovered():
		pill = pill.Background(t.SurfaceHover).Radius(7, 7, 0, 0)
	}
	hovered := pill.Hovered() || active

	pill.Children(func() {
		titleBtn := ui.Button(c, "").Label(title).Height(26).Padding(0, 6).
			Radius(5).Background(ui.Transparent).BorderWidth(0).Shrink(0)
		titleBtn.Children(func() {
			ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
				if s != nil {
					a.stateMark(c, s)
				}
				color := t.TextMuted
				if active {
					color = t.Text
				}
				ui.Text(c, title).FontSize(11.5).
					FontWeight(weightOf(active, 500, 400)).
					TextColor(color).MaxWidth(96).SingleLine().Ellipsis("…")
				if n := leafCount(ws.root); n > 1 {
					ui.Text(c, fmt.Sprintf("×%d", n)).FontSize(9.5).
						TextColor(t.TextMuted.Alpha(0.8))
				}
			})
		})
		titleBtn.PopoverMenu(func(m *ui.Menu) { a.tabMenu(m, ws) })
		if titleBtn.Clicked() {
			a.activateTab(ws)
		}

		closeBtn := ui.Button(c, "").Label("关闭标签页").Padding(3).Radius(5).
			Background(ui.Transparent).BorderWidth(0).Shrink(0)
		if closeBtn.Hovered() {
			closeBtn = closeBtn.Background(t.SurfaceHover)
		}
		closeBtn.Children(func() {
			color := t.TextMuted
			if !hovered {
				color = ui.Transparent
			}
			ui.Icon(c, icoX).FontSize(11).TextColor(color)
		})
		if closeBtn.Clicked() {
			a.closeTab(ws)
		}
	})
	pill.PopoverMenu(func(m *ui.Menu) { a.tabMenu(m, ws) })
}

// cycleTab moves the view to the next (or previous) tab.
func (a *app) cycleTab(step int) {
	if len(a.tabs) == 0 {
		return
	}
	idx := 0
	for i, ws := range a.tabs {
		if ws == a.activeTab {
			idx = i
			break
		}
	}
	a.activateTab(a.tabs[(idx+step+len(a.tabs))%len(a.tabs)])
}

// representativeSession is the session that names a tab: the focused
// pane's, else the first pane with one.
func (a *app) representativeSession(ws *workspace) *engine.Session {
	if ws == nil {
		return nil
	}
	if ws.focus != nil && ws.focus.sessionID != "" {
		if s, ok := a.mgr.Get(ws.focus.sessionID); ok {
			return s
		}
	}
	var found *engine.Session
	var walk func(*paneNode)
	walk = func(n *paneNode) {
		if n == nil || found != nil {
			return
		}
		if n.split == nil {
			if s, ok := a.mgr.Get(n.sessionID); ok {
				found = s
			}
			return
		}
		walk(n.split.first)
		walk(n.split.second)
	}
	walk(ws.root)
	return found
}

// shortSessionTitle names a tab in a few characters: a shell's display
// name rather than its path, and at most 14 runes of a program's own
// title.
func shortSessionTitle(s *engine.Session) string {
	title := strings.TrimSpace(s.Title())
	for _, p := range []string{"管理员: ", "管理员：", "Administrator: "} {
		title = strings.TrimPrefix(title, p)
	}
	if s.Kind() == engine.Local {
		switch s.Shell() {
		case shells.Cmd:
			title = "cmd"
		case shells.PowerShell:
			title = "PowerShell"
		case shells.Pwsh:
			title = "pwsh"
		case shells.GitBash:
			title = "Git Bash"
		}
	}
	if base := filepath.Base(title); strings.EqualFold(filepath.Ext(base), ".exe") {
		title = strings.TrimSuffix(base, filepath.Ext(base))
	}
	if title == "" {
		return "终端"
	}
	runes := []rune(title)
	if len(runes) > 14 {
		title = string(runes[:13]) + "…"
	}
	return title
}

// leafCount counts the panes in a tab.
func leafCount(n *paneNode) int {
	if n == nil {
		return 0
	}
	if n.split == nil {
		return 1
	}
	return leafCount(n.split.first) + leafCount(n.split.second)
}

// tabSessions lists the sessions of a tab, pending panes aside.
func (a *app) tabSessions(ws *workspace) []*engine.Session {
	var out []*engine.Session
	var walk func(*paneNode)
	walk = func(n *paneNode) {
		if n == nil {
			return
		}
		if n.split == nil {
			if s, ok := a.mgr.Get(n.sessionID); ok {
				out = append(out, s)
			}
			return
		}
		walk(n.split.first)
		walk(n.split.second)
	}
	walk(ws.root)
	return out
}

// closeTab closes every session of a tab, refusing while an agent
// operates one of them; the tab goes when its last pane does.
func (a *app) closeTab(ws *workspace) {
	if ws == nil {
		return
	}
	for _, s := range a.tabSessions(ws) {
		if on, _ := s.AgentLock(); on {
			a.pendingToast = "标签页内有会话正被 AI Agent 操作，已阻止关闭"
			a.refreshUI()
			return
		}
	}
	a.dropTab(ws)
	for _, s := range a.tabSessions(ws) {
		s.Close()
	}
}

// tabMenu is a workspace tab's menu: a new tab, and closing this one or
// the others.
func (a *app) tabMenu(m *ui.Menu, ws *workspace) {
	if m.Item("新建标签页").Shortcut(ui.Cmd|ui.Shift, ui.KeyT).Chosen() {
		a.newSession(a.defaultShellID(), "", "")
	}
	m.Separator()
	n := leafCount(ws.root)
	if m.Item("关闭标签页").Disabled(n == 0).Chosen() {
		a.closeTab(ws)
	}
	if m.Item("关闭其他标签页").Disabled(len(a.tabs) < 2).Chosen() {
		for _, other := range append([]*workspace(nil), a.tabs...) {
			if other != ws {
				a.closeTab(other)
			}
		}
	}
}

// paneCard is one pane: a rounded card with the session's header over its
// terminal, brighter with a persimmon edge while it is the focused one.
func (a *app) paneCard(c *ui.Context, n *paneNode) ui.Element {
	t := c.Theme()
	var session *engine.Session
	if n.sessionID != "" {
		session, _ = a.mgr.Get(n.sessionID)
	}
	focused := n == a.focus && n.sessionID != ""

	bg, border := t.Surface, t.Border
	if !t.Dark {
		bg = ui.RGBA(255, 255, 255, 1)
	}
	if focused {
		bg, border = paneCardFocused(c), t.Accent.Alpha(0.55)
	}
	card := ui.Column(c.Key("pane-"+n.sessionID)).Radius(paneRadius).Clip().
		MinWidth(0).MinHeight(0).Background(bg).Border(1, border)
	card.Children(func() {
		a.paneHeader(c, n, session, focused)
		body := ui.Box(c).Grow(1).MinHeight(0).Padding(0, 8, 8, 8)
		body.Children(func() {
			switch {
			case n.sessionID == "":
				ui.Column(c).Fill().Center().Gap(8).Children(func() {
					ui.Spinner(c)
					ui.Text(c, "正在启动…").FontSize(12).TextColor(t.TextMuted)
				})
			case session == nil || session.Terminal() == nil:
				ui.Column(c).Fill().Center().Children(func() {
					ui.Text(c, "会话已关闭").FontSize(12).TextColor(t.TextMuted)
				})
			default:
				v := terminal.View(c, session.Terminal()).Fill()
				if focused {
					v = v.AutoFocus()
				}
				if on, tool := session.AgentLock(); on {
					a.agentMask(c, tool, session.Title())
				}
			}
		})
	})
	if card.Clicked() {
		a.focus = n
		a.activeID = n.sessionID
	}
	card.PopoverMenu(func(m *ui.Menu) { a.paneMenu(m, n) })
	return card
}

// shortPath is a directory as a pane header shows it: the home directory
// as ~ and the last part of a longer path.
func shortPath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if p == home {
			return "~"
		}
		if rest, ok := strings.CutPrefix(p, home+string(filepath.Separator)); ok {
			return "~" + string(filepath.Separator) + rest
		}
	}
	return filepath.Base(p)
}

// paneCardFocused is the face of the pane with the keyboard: a touch
// brighter than its neighbours, so the active one reads at a glance.
func paneCardFocused(c *ui.Context) ui.Color {
	if c.Theme().Dark {
		return ui.RGBA(30, 33, 39, 1)
	}
	return ui.RGBA(255, 255, 255, 1)
}

// paneTextColor is a pane title's ink: full strength while focused, muted
// otherwise.
func paneTextColor(t *ui.Theme, focused bool) ui.Color {
	if focused {
		return t.Text
	}
	return t.TextMuted
}

// paneHeader is the strip over a pane's terminal: the session's state,
// icon and title at left, its split and close buttons at right.
func (a *app) paneHeader(c *ui.Context, n *paneNode, session *engine.Session, focused bool) {
	t := c.Theme()
	h := ui.Row(c).FillWidth().Height(paneHeaderHeight).Padding(0, 8, 0, 10).
		Gap(7).AlignItems(ui.Center).MinWidth(0).Shrink(0)
	if h.DoubleClicked() {
		a.focus = n
		a.toggleZoom(n)
	}
	if h.Clicked() {
		a.focus = n
		a.activeID = n.sessionID
	}
	h.Children(func() {
		if session != nil {
			a.stateMark(c, session)
			icon := icoTerminal
			if session.Kind() == engine.SSH {
				icon = icoGlobe
			}
			ui.Icon(c, icon).FontSize(13).TextColor(t.TextMuted).Shrink(0)
			// The name and directory give way to the buttons: a header
			// narrows by ellipsis, never by hiding a control.
			ui.Row(c).Grow(1).MinWidth(0).Gap(5).AlignItems(ui.Center).
				ClipX().Children(func() {
				title := session.Title()
				if title == "" {
					title = "新分格"
				}
				ui.Text(c, title).FontSize(12).FontWeight(weightOf(focused, 600, 500)).
					TextColor(paneTextColor(t, focused)).
					SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
				if cwd := session.Cwd(); cwd != "" && session.Kind() == engine.Local {
					ui.Text(c, "·").FontSize(11).TextColor(t.TextMuted.Alpha(0.5)).Shrink(0)
					ui.Text(c, shortPath(cwd)).FontSize(11).TextColor(t.TextMuted).
						SingleLine().Ellipsis("…").Shrink(1).MinWidth(0).MaxWidth(110)
				}
			})
		} else {
			ui.Icon(c, icoTerminal).FontSize(13).TextColor(t.TextMuted).Shrink(0)
			ui.Row(c).Grow(1).MinWidth(0).ClipX().Children(func() {
				ui.Text(c, "正在启动…").FontSize(12).TextColor(t.TextMuted).
					SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
			})
		}
		ui.Row(c).Gap(1).AlignItems(ui.Center).Shrink(0).Children(func() {
			if session != nil {
				if paneBtn(c, icoSplitRight, "向右分屏", "向右分屏").Clicked() {
					a.focus = n
					a.splitFocused(true)
				}
				if paneBtn(c, icoSplitDown, "向下分屏", "向下分屏").Clicked() {
					a.focus = n
					a.splitFocused(false)
				}
				zoomIcon, zoomTip := icoZoom, "放大分格"
				if a.zoom == n {
					zoomIcon, zoomTip = icoUnzoom, "还原分格"
				}
				if paneBtn(c, zoomIcon, zoomTip, zoomTip).Clicked() {
					a.toggleZoom(n)
				}
			}
			closeTip := "关闭分格"
			if session != nil {
				if on, _ := session.AgentLock(); on {
					closeTip = "关闭分格（Agent 操作中）"
				}
			}
			closeBtn := paneBtn(c, icoX, closeTip, closeTip)
			if session != nil {
				if on, _ := session.AgentLock(); on {
					closeBtn = closeBtn.Disabled(true)
				}
			}
			if closeBtn.Clicked() {
				a.closePane(n, false)
			}
		})
	})
}

// paneBtn is one of a pane header's quiet icon buttons.
func paneBtn(c *ui.Context, svg *ui.SVG, label, tip string) ui.Element {
	t := c.Theme()
	b := ui.Button(c, "").Label(label).Width(26).Height(26).Padding(0).Radius(7).
		Shrink(0).Background(ui.Transparent).BorderWidth(0)
	color := t.TextMuted
	if b.Hovered() {
		b = b.Background(t.SurfaceHover)
		color = t.Text
	}
	b.Tooltip(tip).Children(func() {
		ui.Icon(c, svg).FontSize(13).TextColor(color)
	})
	return b
}

// agentMask covers a pane an AI agent is operating: a translucent veil
// with a card, so the human watches live while nothing typed reaches the
// session. AutoFocus pulls the focus away from the terminal the moment
// the mask appears.
func (a *app) agentMask(c *ui.Context, tool, target string) {
	t := c.Theme()
	ui.Box(c).Absolute().Top(0).Bottom(0).Left(0).Right(0).
		Background(colorMask).AutoFocus().
		Children(func() {
			ui.Column(c).Fill().Center().Children(func() {
				ui.Column(c).Width(340).Padding(18).Gap(10).Radius(12).
					Background(t.Surface).Border(1, t.Border).
					Shadow(0, 18, 40, -6, ui.RGBA(0, 0, 0, 0.45)).
					Children(func() {
						ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
							ui.Icon(c, icoBot).FontSize(16).TextColor(t.Accent)
							ui.Text(c, "AI Agent 操作中").FontSize(13.5).FontWeight(600)
							ui.Spacer(c)
							dot(c, t.Warning, 5)
						})
						ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
							chip(c, tool, t.Accent, t.Accent.Alpha(0.14))
							ui.Text(c, target).FontSize(10.5).TextColor(t.TextMuted).
								SingleLine().Ellipsis("…")
						})
						ui.Text(c, "输入已锁定，结束后自动恢复。").
							FontSize(11.5).TextColor(t.TextMuted)
						link := ui.Row(c).Gap(5).AlignItems(ui.Center)
						link.Children(func() {
							ui.Text(c, "查看本次调用审计").FontSize(10.5).
								FontWeight(500).TextColor(t.Accent)
							ui.Icon(c, icoArrowRight).FontSize(11).TextColor(t.Accent)
						})
						if link.Clicked() {
							a.openAudit()
						}
					})
			})
		})
}

// toggleZoom shows one pane over the whole area, or gives the split back:
// the same as a double click on a pane's header.
func (a *app) toggleZoom(n *paneNode) {
	if a.zoom != nil {
		a.zoom = nil
		return
	}
	if n == nil || n.sessionID == "" {
		return
	}
	a.zoom = n
	a.focus = n
	a.activeID = n.sessionID
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
	if a.zoom != nil {
		if m.Item("还原分格").Chosen() {
			a.zoom = nil
		}
	} else if n.sessionID != "" {
		if m.Item("放大分格").Chosen() {
			a.toggleZoom(n)
		}
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

// ---- audit window ----

// auditView is the audit window: the filters over the calls, the list of
// what the agent did, and the details of the chosen call beside it.
func (a *app) auditView(c *ui.Context) {
	webTheme(c)
	c.SetMenuPresentation(ui.PopoverMenus)
	t := c.Theme()
	var entries []audit.Entry
	total := int64(0)
	if a.auditLog != nil {
		entries, _ = a.auditLog.Tail(200)
		total = a.auditLog.Written()
	}
	// Tail reads newest first, as a log reads
	shown := a.filterAudit(entries)
	sel := a.selectedAudit(shown)

	ui.Column(c).Fill().Background(t.Background).Children(func() {
		a.auditTitleStrip(c, total, len(shown))
		ui.Divider(c)
		a.auditFilters(c, len(shown), len(entries))
		ui.Divider(c)
		ui.Row(c).Fill().Children(func() {
			ui.Column(c).Fill().Children(func() {
				a.auditHeadRow(c)
				if len(shown) == 0 {
					ui.Column(c).Fill().Center().Gap(8).Children(func() {
						ui.Icon(c, icoHistory).FontSize(26).TextColor(t.TextMuted)
						ui.Text(c, a.auditEmptyText()).FontSize(12).TextColor(t.TextMuted)
					})
					return
				}
				ui.Scroll(c).Fill().Children(func() {
					ui.Column(c).FillWidth().Children(func() {
						for _, e := range shown {
							a.auditRow(c, e, sel)
						}
					})
				})
			})
			ui.Box(c).Width(1).FillHeight().Background(t.Border)
			a.auditDetail(c, sel)
		})
	})
	// The window repaints with fresh entries on its own.
	c.After(2 * time.Second)
}

// auditTitleStrip is the window's own title bar.
func (a *app) auditTitleStrip(c *ui.Context, total int64, shown int) {
	t := c.Theme()
	titleStrip(c, "MCP 审计日志", func() {
		chip(c, fmt.Sprintf("%d 条", total), t.TextMuted, t.Background)
		ui.Text(c, fmt.Sprintf("当前显示 %d 条", shown)).
			FontSize(10.5).TextColor(t.TextMuted)
	})
}

// auditFilters is the row of result filters, the search field and the
// copy-all action.
func (a *app) auditFilters(c *ui.Context, shown, loaded int) {
	t := c.Theme()
	kinds := []struct{ id, label string }{
		{"all", "全部"}, {"ok", "成功"}, {"error", "失败"}, {"denied", "被拦截"},
	}
	ui.Row(c).FillWidth().Height(48).Padding(0, 16).Gap(8).
		AlignItems(ui.Center).Background(t.Background).Children(func() {
		for _, k := range kinds {
			active := a.auditFilter == k.id || (a.auditFilter == "" && k.id == "all")
			b := ui.Button(c, k.label).Height(26).Padding(0, 10).Radius(6).
				FontSize(10.5).Shrink(0)
			if active {
				b = b.Background(t.Surface).Border(1, t.Border).FontWeight(500)
			} else {
				b = b.Background(ui.Transparent).BorderWidth(0)
			}
			if b.Clicked() {
				a.auditFilter = k.id
			}
		}
		ui.Row(c).Width(220).Height(26).Padding(0, 8).Gap(6).Radius(6).
			Background(t.Background).Border(1, t.Border).
			AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icoSearch).FontSize(11).TextColor(t.TextMuted)
			ui.TextInput(c.Key("audit-search"), &a.auditQuery).Grow(1).Height(22).
				FontSize(10.5).BorderWidth(0).Background(ui.Transparent).
				Placeholder("搜索命令、客户端").PlaceholderColor(t.TextMuted)
		})
		ui.Spacer(c)
		ui.Text(c, fmt.Sprintf("%d / %d", shown, loaded)).
			Font(monoFamily()).FontSize(10).TextColor(t.TextMuted)
		if ui.Button(c, "复制筛选结果").Height(26).Padding(0, 9).Radius(6).
			FontSize(10.5).Background(t.Surface).Border(1, t.Border).
			Tooltip("把当前筛选结果以 JSONL 复制到剪贴板").
			Clicked() {
			a.copyAudit()
		}
	})
}

// auditHeadRow is the list's column captions, aligned with the rows.
func (a *app) auditHeadRow(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).FillWidth().Height(30).Padding(0, 16).Gap(10).
		Background(t.Surface).AlignItems(ui.Center).Children(func() {
		cols := []struct {
			label string
			w     float32
			grow  bool
		}{
			{"时间", 62, false}, {"", 14, false}, {"工具", 124, false},
			{"命令", 0, true}, {"客户端", 86, false}, {"耗时", 46, false},
		}
		for _, col := range cols {
			txt := ui.Text(c, col.label).FontSize(9.5).FontWeight(500).
				TextColor(t.TextMuted)
			if col.grow {
				txt.Grow(1)
			} else {
				txt.Width(col.w)
			}
		}
	})
}

// auditRow is one call: time, outcome, tool, command, client, duration.
func (a *app) auditRow(c *ui.Context, e audit.Entry, sel *audit.Entry) {
	t := c.Theme()
	key := auditKey(e)
	selected := sel != nil && auditKey(*sel) == key

	ico, color := icoCheck, t.Success
	switch {
	case e.Guard != "" && strings.HasPrefix(e.Guard, "denied"):
		ico, color = icoShieldX, t.Danger
	case !e.OK:
		ico, color = icoCircleX, t.Danger
	case e.Guard == "confirmed":
		ico, color = icoShield, t.Warning
	}

	row := ui.Row(c.Key(key)).FillWidth().Height(34).Padding(0, 16).Gap(10).
		AlignItems(ui.Center)
	switch {
	case selected:
		row = row.Background(t.SurfaceHover)
	case row.Hovered():
		row = row.Background(t.Surface)
	}
	row.Children(func() {
		mono(c, e.Time.Format("15:04:05")).FontSize(10.5).
			TextColor(t.TextMuted).Width(62)
		ui.Icon(c, ico).FontSize(13).TextColor(color)
		mono(c, e.Tool).FontSize(10.5).TextColor(t.Text).Width(124).
			SingleLine().Ellipsis("…")
		cmd := e.Command
		if cmd == "" {
			cmd = "—"
		}
		mono(c, cmd).FontSize(10.5).TextColor(t.TextMuted).Grow(1).
			SingleLine().Ellipsis("…")
		ui.Text(c, orDash(e.Client)).FontSize(10).TextColor(t.TextMuted).Width(86).
			SingleLine().Ellipsis("…")
		mono(c, fmt.Sprintf("%dms", e.DurationMS)).FontSize(10).
			TextColor(t.TextMuted).Width(46)
	})
	row.PopoverMenu(func(m *ui.Menu) {
		if e.Command != "" {
			if m.Item("复制命令").Chosen() {
				mygo.Clipboard.WriteText(e.Command)
			}
		}
		if e.Err != "" {
			if m.Item("复制错误").Chosen() {
				mygo.Clipboard.WriteText(e.Err)
			}
		}
	})
	if row.Clicked() {
		a.auditSel = key
	}
}

// auditDetail shows everything the list had to shorten about one call.
func (a *app) auditDetail(c *ui.Context, e *audit.Entry) {
	t := c.Theme()
	ui.Column(c).Width(330).FillHeight().Background(t.Surface).
		Padding(16).Gap(12).Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "调用详情").FontSize(12.5).FontWeight(600)
			ui.Spacer(c)
			if e != nil {
				if ui.Button(c, "").Label("复制 JSONL").Padding(4).Radius(5).
					Background(ui.Transparent).BorderWidth(0).Tooltip("复制这条记录").Children(func() {
					ui.Icon(c, icoCopy).FontSize(12).TextColor(t.TextMuted)
				}).Clicked() {
					if line, err := json.Marshal(e); err == nil {
						mygo.Clipboard.WriteText(string(line))
					}
				}
			}
		})
		if e == nil {
			ui.Column(c).Fill().Center().Gap(8).Children(func() {
				ui.Icon(c, icoShield).FontSize(22).TextColor(t.TextMuted)
				ui.Text(c, "选择一条记录查看详情").FontSize(11.5).TextColor(t.TextMuted)
			})
			return
		}

		outcome, color := "成功", t.Success
		switch {
		case e.Guard != "" && strings.HasPrefix(e.Guard, "denied"):
			outcome, color = "被护栏拦截", t.Danger
		case !e.OK:
			outcome, color = "执行失败", t.Danger
		case e.Guard == "confirmed":
			outcome, color = "人工确认后放行", t.Warning
		}
		ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
			dot(c, color, 6)
			ui.Text(c, outcome).FontSize(11.5).FontWeight(500).TextColor(color)
		})

		metaRow(c, "工具", e.Tool, true)
		metaRow(c, "客户端", orDash(e.Client), false)
		metaRow(c, "目标会话", orDash(e.Session), false)
		metaRow(c, "时间", e.Time.Format("01-02 15:04:05"), true)
		metaRow(c, "耗时", fmt.Sprintf("%d ms", e.DurationMS), true)
		metaRow(c, "传输", orDash(e.Transport), false)

		ui.Divider(c)
		sectionLabel(c, "命令")
		codeBlock(c, func() {
			cmd := e.Command
			if cmd == "" {
				cmd = "（未执行命令）"
			}
			mono(c, cmd).FontSize(10.5).Wrap()
			if e.OutputBytes > 0 {
				mono(c, fmt.Sprintf("输出 %d 字节", e.OutputBytes)).
					FontSize(9.5).TextColor(t.TextMuted)
			}
		})
		if e.Err != "" {
			sectionLabel(c, "错误")
			ui.Text(c, firstLine(e.Err)).FontSize(10.5).TextColor(t.Danger).Wrap()
		}
		if e.Guard != "" {
			sectionLabel(c, "护栏")
			ui.Text(c, guardText(e.Guard)).FontSize(10.5).TextColor(t.Warning).Wrap()
		}

		ui.Spacer(c)
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			if ui.Button(c, "复制命令").Height(30).Padding(0, 12).Radius(6).
				FontSize(11).Grow(1).Background(t.Accent.Alpha(0.14)).
				BorderWidth(0).TextColor(t.Accent).Clicked() && e.Command != "" {
				mygo.Clipboard.WriteText(e.Command)
			}
			if ui.Button(c, "复制 JSONL").Height(30).Padding(0, 12).Radius(6).
				FontSize(11).Grow(1).Background(ui.Transparent).
				Border(1, t.Border).Clicked() {
				if line, err := json.Marshal(e); err == nil {
					mygo.Clipboard.WriteText(string(line))
				}
			}
		})
	})
}

// metaRow is a label and its value in the audit detail.
func metaRow(c *ui.Context, label, value string, code bool) {
	t := c.Theme()
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(10.5).TextColor(t.TextMuted).Width(56)
		txt := ui.Text(c, value).FontSize(11).TextColor(t.Text).
			SingleLine().Ellipsis("…").Grow(1)
		if code {
			txt.Font(monoFamily())
		}
	})
}

// filterAudit narrows the trail by outcome and by a search over tool,
// command and client.
func (a *app) filterAudit(entries []audit.Entry) []audit.Entry {
	q := strings.ToLower(strings.TrimSpace(a.auditQuery))
	out := make([]audit.Entry, 0, len(entries))
	for _, e := range entries {
		denied := strings.HasPrefix(e.Guard, "denied")
		switch a.auditFilter {
		case "ok":
			if !e.OK || denied {
				continue
			}
		case "error":
			if e.OK {
				continue
			}
		case "denied":
			if !denied {
				continue
			}
		}
		if q != "" {
			hay := strings.ToLower(e.Tool + " " + e.Command + " " + e.Client + " " + e.Err)
			if !strings.Contains(hay, q) {
				continue
			}
		}
		out = append(out, e)
	}
	return out
}

// selectedAudit is the entry the detail panel shows: the chosen one, or
// the newest of the filtered list.
func (a *app) selectedAudit(entries []audit.Entry) *audit.Entry {
	if len(entries) == 0 {
		return nil
	}
	for i := range entries {
		if auditKey(entries[i]) == a.auditSel {
			return &entries[i]
		}
	}
	return &entries[0]
}

// auditKey names one call for selection, stable across refreshes.
func auditKey(e audit.Entry) string {
	return fmt.Sprintf("%d-%s", e.Time.UnixNano(), e.Tool)
}

// guardText reads a guard verdict in words.
func guardText(g string) string {
	switch {
	case g == "":
		return ""
	case g == "confirmed":
		return "命中规则，人工确认后放行"
	case strings.HasPrefix(g, "denied:"):
		return "已拦截：" + strings.TrimPrefix(g, "denied:")
	default:
		return g
	}
}

// auditEmptyText explains an empty list.
func (a *app) auditEmptyText() string {
	if (a.auditFilter != "" && a.auditFilter != "all") || strings.TrimSpace(a.auditQuery) != "" {
		return "没有符合筛选条件的调用"
	}
	return "还没有 MCP 调用记录"
}

// copyAudit puts the filtered trail on the clipboard as JSON lines.
func (a *app) copyAudit() {
	if a.auditLog == nil {
		return
	}
	entries, _ := a.auditLog.Tail(200)
	shown := a.filterAudit(entries)
	var b strings.Builder
	for _, e := range shown {
		if line, err := json.Marshal(e); err == nil {
			b.Write(line)
			b.WriteByte('\n')
		}
	}
	mygo.Clipboard.WriteText(b.String())
	a.pendingToast = fmt.Sprintf("已复制 %d 条审计记录", len(shown))
	if a.auditWin != nil {
		a.auditWin.Invalidate()
	}
}

// ---- helpers ----

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

func (a *app) active() *engine.Session {
	s, _ := a.mgr.Get(a.activeID)
	return s
}

// sessionsOf lists the sessions of a kind, narrowed by the sidebar filter.
func (a *app) sessionsOf(kind engine.Kind) []*engine.Session {
	q := strings.ToLower(strings.TrimSpace(a.filter))
	var out []*engine.Session
	for _, s := range a.mgr.List() {
		if s.Kind() != kind {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(s.Title()), q) {
			continue
		}
		out = append(out, s)
	}
	return out
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

// titleStrip is a secondary window's own title bar (TitleBarHidden): a
// quiet band with the title at left and extra controls at right, the
// whole strip dragging the window and keeping clear of the system's
// window controls.
func titleStrip(c *ui.Context, title string, right func()) {
	bar := c.TitleBar()
	t := c.Theme()
	ui.Row(c).FillWidth().Height(chromeBarHeight(c)).Padding(0, bar.Right+12, 0, bar.Left+14).
		Background(t.Surface).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).
		DragWindow().AlignItems(ui.Center).Gap(9).Children(func() {
		if title != "" {
			ui.Text(c, title).FontSize(13).FontWeight(600)
		}
		ui.Spacer(c)
		if right != nil {
			right()
		}
	})
}

// popoutView is a synchronized popout window: its own title strip over
// the session's mirror terminal.
func (a *app) popoutView(c *ui.Context, mv *engine.MirrorView, title string) {
	webTheme(c)
	c.SetMenuPresentation(ui.PopoverMenus)
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		titleStrip(c, title+" — 同屏", nil)
		if term := mv.Terminal(); term != nil {
			terminal.View(c, term).Fill().AutoFocus()
			return
		}
		ui.Column(c).Fill().Center().Children(func() {
			ui.Text(c, "会话已结束").FontSize(12).TextColor(t.TextMuted)
		})
	})
}
