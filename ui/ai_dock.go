// The AI operator's dock on the right of the content area, and the
// title-bar button that opens and closes it (design boards 08 and 09).
//
// The dock is a split pane: the divider resizes it between 280 and 420
// DIPs, the width persists in the settings, and the timeline lists the
// agent's operations as they stream in.

package ui

import (
	"fmt"

	"adakaki/internal/store"

	myui "github.com/egoist/mygo/ui"
)

// aiOp is one timeline entry: an assistant message (kind "msg") or a tool
// call the agent made (kind "op"), with its one-line command and result.
type aiOp struct {
	kind string // "msg" | "op"
	tool string
	cmd  string
	out  string
	err  string
	ok   bool
	ms   int64
}

// clampAIDockWidth keeps the dock inside the documented 280–420 range.
func clampAIDockWidth(v float32) float32 {
	if v < float32(store.AIDockWidthMin) {
		return float32(store.AIDockWidthMin)
	}
	if v > float32(store.AIDockWidthMax) {
		return float32(store.AIDockWidthMax)
	}
	return v
}

// aiDockVisible reports whether the dock takes room in the content area.
func (a *app) aiDockVisible() bool {
	return a.store.Settings.AI.Enabled && a.aiDockOpen
}

// toggleAIDock flips the dock and persists the choice.
func (a *app) toggleAIDock() {
	a.setAIDock(!a.aiDockOpen, true)
}

// setAIDock sets the open state; persist writes it to config.json and
// repaints, while a test may pass false to keep the user's file untouched.
func (a *app) setAIDock(open, persist bool) {
	a.aiDockOpen = open
	if !persist {
		return
	}
	v := open
	a.store.Settings.AI.DockOpen = &v
	a.saveSettingsNow()
	a.refreshUI()
}

// aiButton is the title-bar toggle: ghost at rest, tinted while open, with
// the step badge when there is something to show (board 09's four states).
func (a *app) aiButton(c *myui.Context) {
	if !a.store.Settings.AI.Enabled {
		return
	}
	t := c.Theme()
	b := myui.Button(c, "").Label("AI 操作").Height(26).Padding(0, 8).Radius(7).
		Shrink(0).BorderWidth(0).Background(myui.Transparent).
		Tooltip("展开/收起 AI 操作栏（Ctrl+Alt+A）")
	color := t.TextMuted
	if b.Hovered() && !a.aiDockOpen {
		b = b.Background(t.SurfaceHover)
		color = t.Text
	}
	if a.aiDockOpen {
		b = b.Background(t.Accent.Alpha(0.14)).BorderWidth(1).
			BorderColor(t.Accent.Alpha(0.32))
		color = t.Accent
	}
	b.Children(func() {
		myui.Row(c).Gap(5).AlignItems(myui.Center).Children(func() {
			myui.Icon(c, icoMessage).FontSize(13).TextColor(color)
			myui.Text(c, "AI 操作").FontSize(11).FontWeight(500).TextColor(color)
			if n := a.aiStepCount(); n > 0 {
				count := fmt.Sprint(n)
				if n > 99 {
					count = "99+"
				}
				myui.Box(c).Height(14).Padding(0, 3).Radius(4).
					Background(t.Accent).AlignItems(myui.Center).Children(func() {
					mono(c, count).FontSize(9).TextColor(t.AccentText)
				})
			}
		})
	})
	if b.Clicked() {
		a.toggleAIDock()
	}
}

// aiDock is the right column itself: head, target line, timeline, composer.
func (a *app) aiDock(c *myui.Context) {
	t := c.Theme()
	myui.Column(c).Fill().Direction(myui.LeftToRight).Background(t.Surface).
		BorderWidth(0, 0, 0, 1).BorderColor(t.Border).Children(func() {
		a.aiDockHead(c, t)
		a.aiDockTarget(c, t)
		a.aiDockTimeline(c, t)
		a.aiComposer(c, t)
	})
}

// aiDockHead is the 36 DIP strip: icon, title, target chip, steps, fold.
func (a *app) aiDockHead(c *myui.Context, t *myui.Theme) {
	myui.Row(c).FillWidth().Height(36).Padding(0, 6, 0, 12).Gap(7).
		AlignItems(myui.Center).Children(func() {
		myui.Icon(c, icoMessage).FontSize(14).TextColor(t.Accent)
		myui.Text(c, "AI 操作").FontSize(12).FontWeight(600).TextColor(t.Text)
		if s := a.active(); s != nil {
			chip(c, s.Title(), t.TextMuted, t.Background)
		}
		myui.Spacer(c)
		if n := a.aiStepCount(); n > 0 {
			chip(c, fmt.Sprintf("%d 步", n), t.TextMuted, t.Background)
		}
		ghostButton(c, icoChevronRight, "收起 AI 操作栏", "收起（Ctrl+Alt+A）",
			a.toggleAIDock)
	})
}

// aiDockTarget is the 24 DIP line naming what the agent is working on.
func (a *app) aiDockTarget(c *myui.Context, t *myui.Theme) {
	state, _, _, _, running := a.aiSnapshot()
	color := t.TextMuted
	label := "选择一个会话后开始"
	if s := a.active(); s != nil {
		label = "目标会话 " + s.ID()
		if on, tool := s.AgentLock(); on {
			color = t.Success
			label = "正在执行 " + tool + " · " + s.ID()
		}
	}
	switch {
	case state == "thinking":
		color = t.Accent
		label = "模型思考中…"
	case state == "tool" || running:
		color = t.Success
	}
	myui.Row(c).FillWidth().Height(24).Padding(0, 12).Gap(6).
		AlignItems(myui.Center).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).
		Children(func() {
			dot(c, color, 5)
			myui.Text(c, label).FontSize(10.5).TextColor(t.TextMuted).
				SingleLine().Ellipsis("…").Grow(1)
			if myui.Button(c, "").Label("审计").Height(20).Padding(0, 6).Radius(5).
				FontSize(10).Background(myui.Transparent).BorderWidth(0).
				Children(func() {
					myui.Text(c, "审计").FontSize(10).FontWeight(500).
						TextColor(t.Accent)
					myui.Icon(c, icoChevronRight).FontSize(10).TextColor(t.Accent)
				}).Clicked() {
				a.openAudit()
			}
		})
}

// aiDockTimeline lists the operations, or explains the empty state.
func (a *app) aiDockTimeline(c *myui.Context, t *myui.Theme) {
	state, errText, _, ops, running := a.aiSnapshot()
	myui.Scroll(c).Fill().Padding(10, 12).Children(func() {
		if len(ops) == 0 && !running && errText == "" {
			myui.Column(c).FillWidth().Padding(6, 0).Gap(8).
				AlignItems(myui.Center).Children(func() {
				myui.Icon(c, icoMessage).FontSize(20).TextColor(t.TextMuted.Alpha(0.7))
				myui.Text(c, "还没有操作").FontSize(12).TextColor(t.TextMuted)
				myui.Text(c, "在下面的输入框里让它做点什么；它的每一步工具调用都会按顺序列在这里，并带审计入口。").
					FontSize(10.5).TextColor(t.TextMuted.Alpha(0.9)).Wrap()
			})
			return
		}
		for _, op := range ops {
			a.aiOpEntry(c, t, op)
		}
		if state == "thinking" || state == "tool" {
			myui.Row(c).FillWidth().Gap(7).AlignItems(myui.Center).Padding(4, 2).
				Children(func() {
					myui.Spinner(c)
					myui.Text(c, "正在"+map[string]string{"thinking": "思考", "tool": "执行"}[state]+"…").
						FontSize(10.5).TextColor(t.TextMuted)
				})
		}
		if errText != "" {
			myui.Column(c).FillWidth().Padding(8, 9).Gap(4).Radius(9).
				Background(t.Warning.Alpha(0.10)).BorderWidth(1).
				BorderColor(t.Warning.Alpha(0.35)).Children(func() {
				myui.Row(c).Gap(6).AlignItems(myui.Center).Children(func() {
					myui.Icon(c, icoAlert).FontSize(12).TextColor(t.Warning)
					myui.Text(c, "需要你处理").FontSize(10.5).FontWeight(500).
						TextColor(t.Warning)
				})
				myui.Text(c, errText).FontSize(10.5).TextColor(t.TextMuted).Wrap()
			})
		}
	})
}

// aiOpEntry renders one timeline entry: a message bubble or a tool card.
func (a *app) aiOpEntry(c *myui.Context, t *myui.Theme, op aiOp) {
	if op.kind == "msg" {
		myui.Column(c).FillWidth().Padding(8, 9).Gap(4).Radius(9).
			Background(t.Accent.Alpha(0.08)).BorderWidth(1).
			BorderColor(t.Accent.Alpha(0.22)).Children(func() {
			myui.Row(c).Gap(5).AlignItems(myui.Center).Children(func() {
				myui.Icon(c, icoMessage).FontSize(11).TextColor(t.Accent)
				myui.Text(c, "AI").FontSize(10).FontWeight(500).TextColor(t.Accent)
			})
			myui.Text(c, op.out).FontSize(11.5).TextColor(t.Text).Wrap()
		})
		return
	}

	myui.Column(c).FillWidth().Padding(8, 9).Gap(5).Radius(9).
		Background(t.Background).BorderWidth(1).BorderColor(t.Border).
		Children(func() {
			myui.Row(c).FillWidth().Gap(6).AlignItems(myui.Center).Children(func() {
				icon, color, label := icoCheck, t.Success, "完成"
				if !op.ok {
					icon, color, label = icoAlert, t.Warning, "未完成"
				}
				myui.Icon(c, icon).FontSize(12).TextColor(color)
				mono(c, op.tool).FontSize(10).TextColor(t.Accent)
				myui.Spacer(c)
				chip(c, label, color, t.Surface)
			})
			if op.cmd != "" {
				mono(c, op.cmd).FontSize(10).TextColor(t.Text).SingleLine().Ellipsis("…")
			}
			if op.out != "" {
				myui.Text(c, op.out).FontSize(10).TextColor(t.TextMuted).
					SingleLine().Ellipsis("…")
			} else if op.err != "" {
				myui.Text(c, op.err).FontSize(10).TextColor(t.Warning).
					SingleLine().Ellipsis("…")
			}
			myui.Row(c).FillWidth().Gap(6).AlignItems(myui.Center).Children(func() {
				if op.ms > 0 {
					myui.Text(c, fmt.Sprintf("%dms", op.ms)).FontSize(9.5).
						TextColor(t.TextMuted)
				}
				myui.Spacer(c)
				if myui.Button(c, "").Label("查看审计").Height(18).Padding(0, 4).
					Radius(4).Background(myui.Transparent).BorderWidth(0).
					Children(func() {
						myui.Text(c, "查看审计").FontSize(9.5).FontWeight(500).
							TextColor(t.Accent)
					}).Clicked() {
					a.openAudit()
				}
			})
		})
}

// aiComposer is the input under the timeline, with the model line beneath.
func (a *app) aiComposer(c *myui.Context, t *myui.Theme) {
	_, _, tokens, _, running := a.aiSnapshot()
	myui.Column(c).FillWidth().Padding(8, 12, 0, 12).Gap(4).
		BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
		myui.Row(c).FillWidth().Height(34).Padding(0, 6, 0, 9).Gap(6).Radius(8).
			Background(t.Background).BorderWidth(1).BorderColor(t.Border).
			AlignItems(myui.Center).Children(func() {
			myui.TextInput(c.Key("ai-dock-input"), &a.aiInput).Grow(1).Height(26).
				FontSize(11.5).BorderWidth(0).Background(myui.Transparent).
				Placeholder("让它做点什么，或继续上一轮…").
				PlaceholderColor(t.TextMuted)
			send := myui.Button(c, "").Label("发送").Width(26).Height(26).
				Padding(0).Radius(7).Background(t.Accent).Children(func() {
				myui.Icon(c, icoSend).FontSize(13).TextColor(t.AccentText)
			})
			if running {
				send = send.Background(t.SurfacePressed)
			}
			if send.Clicked() {
				if running {
					a.pendingToast = "上一轮还在执行，等它结束再发"
					a.refreshUI()
				} else {
					a.startAITurn()
				}
			}
		})
		myui.Row(c).FillWidth().Height(24).Gap(6).AlignItems(myui.Center).
			Children(func() {
				cfg := a.store.Settings.AI
				mono(c, cfg.Model).FontSize(9.5).TextColor(t.TextMuted).
					SingleLine().Ellipsis("…")
				myui.Text(c, fmt.Sprintf("· tokens %d", tokens)).FontSize(9.5).
					TextColor(t.TextMuted)
				myui.Spacer(c)
			})
	})
}
