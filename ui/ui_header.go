// The window-wide chrome above and below the body: the title strip
// with the session context and window actions, and the status bar.

package ui

import (
	"adakaki/internal/engine"
	"fmt"

	"github.com/egoist/mygo"
	myui "github.com/egoist/mygo/ui"
)

// appHeader is the window's one title strip: the brand and the rail's
// collapse button at left, the active session's context in the middle,
// the agent's presence and the window actions at right. It runs across
// the sidebar and the content, and stays whole as the sidebar folds.
func (a *app) appHeader(c *myui.Context) {
	bar := c.TitleBar()
	t := c.Theme()
	myui.Row(c).FillWidth().Height(chromeBarHeight(c)).
		Padding(0, bar.Right+10, 0, bar.Left+14).
		Background(t.Surface).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).
		DragWindow().AlignItems(myui.Center).Gap(9).Children(func() {
		a.logoMark(c)
		myui.Text(c, "AdaKaki").FontSize(13.5).FontWeight(600)
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
			myui.Text(c, s.Title()).FontSize(12.5).FontWeight(500).
				MaxWidth(280).SingleLine().Ellipsis("…")
			kind := "本地"
			if s.Kind() == engine.SSH {
				kind = "SSH"
			}
			chip(c, kind, t.TextMuted, t.Background)
		} else {
			myui.Text(c, "没有活动会话").FontSize(12).TextColor(t.TextMuted)
		}
		myui.Spacer(c)
		if n, _, _ := a.agentSummary(); n > 0 {
			myui.Row(c).Padding(3, 8).Gap(5).Radius(6).
				Background(t.Warning.Alpha(0.14)).AlignItems(myui.Center).Children(func() {
				myui.Icon(c, icoBot).FontSize(12).TextColor(t.Warning)
				myui.Text(c, fmt.Sprintf("Agent ×%d", n)).FontSize(10.5).
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

// appFooter is the one status strip along the bottom of the whole window:
// the MCP endpoint and its port at left, the session count and the active
// session in the middle, the agent and stdio at right. It stays whole as
// the sidebar folds.
func (a *app) appFooter(c *myui.Context) {
	t := c.Theme()
	port := a.store.Settings.MCPHTTPPort
	if port == 0 {
		port = 8731
	}
	myui.Row(c).FillWidth().Height(statusBarHeight).Padding(0, 12).
		Background(t.Surface).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).
		AlignItems(myui.Center).Gap(9).Children(func() {
		if a.mcpUp {
			dot(c, t.Success, 5)
			myui.Text(c, "MCP 已就绪").FontSize(10.5).TextColor(t.TextMuted)
		} else {
			dot(c, t.TextMuted, 5)
			myui.Text(c, "MCP 未启动").FontSize(10.5).TextColor(t.TextMuted)
		}
		mono(c, fmt.Sprintf(":%d", port)).FontSize(9.5).TextColor(t.TextMuted)
		if myui.Button(c, "").Label("复制端口").Padding(2).Radius(4).
			Background(myui.Transparent).BorderWidth(0).Tooltip("复制 MCP 端口").Children(func() {
			myui.Icon(c, icoCopy).FontSize(11).TextColor(t.TextMuted)
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
		myui.Text(c, fmt.Sprintf("%d 个会话", n)).FontSize(10.5).TextColor(t.TextMuted)
		if s := a.active(); s != nil {
			myui.Text(c, "·").FontSize(10.5).TextColor(t.TextMuted.Alpha(0.5))
			myui.Text(c, s.Title()).FontSize(10.5).TextColor(t.TextMuted).
				MaxWidth(220).SingleLine().Ellipsis("…")
		}
		myui.Spacer(c)
		if agents := a.agentBusy(); agents > 0 {
			myui.Icon(c, icoBot).FontSize(12).TextColor(t.Warning)
			myui.Text(c, fmt.Sprintf("Agent ×%d", agents)).FontSize(10.5).
				FontWeight(500).TextColor(t.Warning)
			verticalRule(c)
		}
		myui.Text(c, "stdio 可用").FontSize(10.5).TextColor(t.TextMuted)
	})
}

// logoMark is the app's mark: a rounded accent tile with a terminal
// glyph punched into it.
func (a *app) logoMark(c *myui.Context) {
	t := c.Theme()
	myui.Box(c).Width(22).Height(22).Radius(6).Background(t.Accent).
		AlignItems(myui.Center).Justify(myui.Center).Shrink(0).Children(func() {
		myui.Icon(c, icoMark).FontSize(16).TextColor(t.AccentText)
	})
}
