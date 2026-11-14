// One pane's card: the header with its state, title, directory and
// split/zoom/close buttons, the terminal body and the agent mask.

package ui

import (
	"adakaki/internal/engine"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/plugins/terminal"
	myui "github.com/egoist/mygo/ui"
)

// paneCard is one pane: a rounded card with the session's header over its
// terminal, brighter with a persimmon edge while it is the focused one.
func (a *app) paneCard(c *myui.Context, n *paneNode) myui.Element {
	t := c.Theme()
	var session *engine.Session
	if n.sessionID != "" {
		session, _ = a.mgr.Get(n.sessionID)
	}
	focused := n == a.focus && n.sessionID != ""

	bg, border := t.Surface, t.Border
	if !t.Dark {
		bg = myui.RGBA(255, 255, 255, 1)
	}
	if focused {
		bg, border = paneCardFocused(c), t.Accent.Alpha(0.55)
	}
	card := myui.Column(c.Key("pane-"+n.sessionID)).Radius(paneRadius).Clip().
		MinWidth(0).MinHeight(0).Background(bg).Border(1, border)
	card.Children(func() {
		a.paneHeader(c, n, session, focused)
		body := myui.Box(c).Grow(1).MinHeight(0).Padding(0, 8, 8, 8)
		body.Children(func() {
			switch {
			case n.sessionID == "":
				myui.Column(c).Fill().Center().Gap(8).Children(func() {
					myui.Spinner(c)
					myui.Text(c, "正在启动…").FontSize(12).TextColor(t.TextMuted)
				})
			case session == nil || session.Terminal() == nil:
				myui.Column(c).Fill().Center().Children(func() {
					myui.Text(c, "会话已关闭").FontSize(12).TextColor(t.TextMuted)
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
	card.PopoverMenu(func(m *myui.Menu) { a.paneMenu(m, n) })
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
func paneCardFocused(c *myui.Context) myui.Color {
	if c.Theme().Dark {
		return myui.RGBA(30, 33, 39, 1)
	}
	return myui.RGBA(255, 255, 255, 1)
}

// paneTextColor is a pane title's ink: full strength while focused, muted
// otherwise.
func paneTextColor(t *myui.Theme, focused bool) myui.Color {
	if focused {
		return t.Text
	}
	return t.TextMuted
}

// paneHeader is the strip over a pane's terminal: the session's state,
// icon and title at left, its split and close buttons at right.
func (a *app) paneHeader(c *myui.Context, n *paneNode, session *engine.Session, focused bool) {
	t := c.Theme()
	h := myui.Row(c).FillWidth().Height(paneHeaderHeight).Padding(0, 8, 0, 10).
		Gap(7).AlignItems(myui.Center).MinWidth(0).Shrink(0)
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
			myui.Icon(c, icon).FontSize(13).TextColor(t.TextMuted).Shrink(0)
			// The name and directory give way to the buttons: a header
			// narrows by ellipsis, never by hiding a control.
			myui.Row(c).Grow(1).MinWidth(0).Gap(5).AlignItems(myui.Center).
				ClipX().Children(func() {
				title := session.Title()
				if title == "" {
					title = "新分格"
				}
				myui.Text(c, title).FontSize(12).FontWeight(weightOf(focused, 600, 500)).
					TextColor(paneTextColor(t, focused)).
					SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
				if cwd := session.Cwd(); cwd != "" && session.Kind() == engine.Local {
					myui.Text(c, "·").FontSize(11).TextColor(t.TextMuted.Alpha(0.5)).Shrink(0)
					myui.Text(c, shortPath(cwd)).FontSize(11).TextColor(t.TextMuted).
						SingleLine().Ellipsis("…").Shrink(1).MinWidth(0).MaxWidth(110)
				}
			})
		} else {
			myui.Icon(c, icoTerminal).FontSize(13).TextColor(t.TextMuted).Shrink(0)
			myui.Row(c).Grow(1).MinWidth(0).ClipX().Children(func() {
				myui.Text(c, "正在启动…").FontSize(12).TextColor(t.TextMuted).
					SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
			})
		}
		myui.Row(c).Gap(1).AlignItems(myui.Center).Shrink(0).Children(func() {
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
func paneBtn(c *myui.Context, svg *myui.SVG, label, tip string) myui.Element {
	t := c.Theme()
	b := myui.Button(c, "").Label(label).Width(26).Height(26).Padding(0).Radius(7).
		Shrink(0).Background(myui.Transparent).BorderWidth(0)
	color := t.TextMuted
	if b.Hovered() {
		b = b.Background(t.SurfaceHover)
		color = t.Text
	}
	b.Tooltip(tip).Children(func() {
		myui.Icon(c, svg).FontSize(13).TextColor(color)
	})
	return b
}

// agentMask covers a pane an AI agent is operating: a translucent veil
// with a card, so the human watches live while nothing typed reaches the
// session. AutoFocus pulls the focus away from the terminal the moment
// the mask appears.
func (a *app) agentMask(c *myui.Context, tool, target string) {
	t := c.Theme()
	myui.Box(c).Absolute().Top(0).Bottom(0).Left(0).Right(0).
		Background(colorMask).AutoFocus().
		Children(func() {
			myui.Column(c).Fill().Center().Children(func() {
				myui.Column(c).Width(340).Padding(18).Gap(10).Radius(12).
					Background(t.Surface).Border(1, t.Border).
					Shadow(0, 18, 40, -6, myui.RGBA(0, 0, 0, 0.45)).
					Children(func() {
						myui.Row(c).FillWidth().Gap(8).AlignItems(myui.Center).Children(func() {
							myui.Icon(c, icoBot).FontSize(16).TextColor(t.Accent)
							myui.Text(c, "AI Agent 操作中").FontSize(13.5).FontWeight(600)
							myui.Spacer(c)
							dot(c, t.Warning, 5)
						})
						myui.Row(c).FillWidth().Gap(6).AlignItems(myui.Center).Children(func() {
							chip(c, tool, t.Accent, t.Accent.Alpha(0.14))
							myui.Text(c, target).FontSize(10.5).TextColor(t.TextMuted).
								SingleLine().Ellipsis("…")
						})
						myui.Text(c, "输入已锁定，结束后自动恢复。").
							FontSize(11.5).TextColor(t.TextMuted)
						link := myui.Row(c).Gap(5).AlignItems(myui.Center)
						link.Children(func() {
							myui.Text(c, "查看本次调用审计").FontSize(10.5).
								FontWeight(500).TextColor(t.Accent)
							myui.Icon(c, icoArrowRight).FontSize(11).TextColor(t.Accent)
						})
						if link.Clicked() {
							a.openAudit()
						}
					})
			})
		})
}
