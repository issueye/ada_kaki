// The session sidebar: the new-session action, the filter, the
// local and SSH trees, the folded rail and the agent card.

package ui

import (
	"adakaki/internal/engine"
	"fmt"
	"strings"

	myui "github.com/egoist/mygo/ui"
)

// sidebarPane is the left column below the shared header: the primary
// action, the session filter and the sessions, with the agent's live work
// at the foot.
func (a *app) sidebarPane(c *myui.Context) {
	t := c.Theme()
	myui.Column(c).Fill().Background(t.Surface).
		BorderWidth(0, 1, 0, 0).BorderColor(t.Border).Children(func() {
		myui.Column(c).Padding(10, 10, 6, 10).Gap(8).Children(func() {
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
func (a *app) sidebarRail(c *myui.Context) {
	t := c.Theme()
	myui.Column(c).Width(sidebarRailWidth).FillHeight().Shrink(0).
		Background(t.Surface).BorderWidth(0, 1, 0, 0).BorderColor(t.Border).
		Children(func() {
			// All spacing lives on the containers: the folded rail has no
			// per-item margins for the layout to lose.
			myui.Column(c).FillWidth().Padding(10, 0, 0, 0).Gap(8).
				AlignItems(myui.Center).Children(func() {
				a.railAction(c, icoPlus, "新建会话", "新建会话（Ctrl+Shift+T）",
					true, a.newSessionMenu, nil)
				a.railAction(c, icoSearch, "搜索会话", "展开侧栏并筛选会话",
					false, nil, func() { a.sidebarCollapsed = false })
			})

			myui.Column(c).FillWidth().Padding(14, 0).AlignItems(myui.Center).
				Children(func() {
					myui.Box(c).Width(28).Height(1).Background(t.Border).Shrink(0)
				})

			myui.Scroll(c).FillWidth().Grow(1).Children(func() {
				myui.Column(c).FillWidth().Padding(0, 0, 8, 0).Gap(6).
					AlignItems(myui.Center).Children(func() {
					for _, s := range a.mgr.List() {
						a.railSession(c, s)
					}
				})
			})
		})
}

// railAction is one folded-sidebar action: 36×36 on the rail's grid,
// filled for the primary action, a hover tint for every other one.
func (a *app) railAction(c *myui.Context, svg *myui.SVG, label, tip string,
	accent bool, build func(*myui.Menu), fn func()) {
	t := c.Theme()
	b := myui.Button(c, "").Label(label).Width(36).Height(36).Radius(10).
		Padding(0).Shrink(0).Background(myui.Transparent).BorderWidth(0)
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
		myui.Icon(c, svg).FontSize(20).TextColor(color)
	})
	if fn != nil && b.Clicked() {
		fn()
	}
}

// railSession is one session in the folded sidebar: a 36×36 chip with its
// type icon, a state dot in the lower right, and a persimmon edge when it
// is the active one.
func (a *app) railSession(c *myui.Context, s *engine.Session) {
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

	b := myui.Button(c.Key("rail-"+s.ID()), "").Label(s.Title()).
		Width(36).Height(36).Radius(10).Padding(0).Shrink(0).
		Background(myui.Transparent).BorderWidth(0)
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
	b.PopoverMenu(func(m *myui.Menu) { a.sessionMenu(m, s) })
	b.Children(func() {
		myui.Icon(c, icon).FontSize(18).TextColor(color)
		if active {
			myui.Box(c).Width(2).Height(16).Radius(1).Background(t.Accent).
				Absolute().Left(4).Top(10).Shrink(0)
		}
		myui.Box(c).Width(6).Height(6).Radius(3).Background(state).
			Absolute().Right(4).Bottom(4).Shrink(0)
	})
	if b.Clicked() {
		a.viewSession(s.ID())
	}
}

// sessionFilter is the sidebar's quiet search field; typing narrows the
// session lists below.
func (a *app) sessionFilter(c *myui.Context) {
	t := c.Theme()
	myui.Row(c).FillWidth().Height(28).Padding(0, 8).Gap(6).Radius(6).
		Background(t.Background).Border(1, t.Border).AlignItems(myui.Center).Children(func() {
		myui.Icon(c, icoSearch).FontSize(12).TextColor(t.TextMuted)
		myui.TextInput(c.Key("session-filter"), &a.filter).Grow(1).Height(24).
			FontSize(11.5).BorderWidth(0).Background(myui.Transparent).
			Placeholder("筛选会话…").PlaceholderColor(t.TextMuted)
		if a.filter != "" {
			if myui.Button(c, "").Label("清除筛选").Padding(1).Radius(4).
				Background(myui.Transparent).BorderWidth(0).Children(func() {
				myui.Icon(c, icoX).FontSize(11).TextColor(t.TextMuted)
			}).Clicked() {
				a.filter = ""
			}
		}
	})
}

// sidebarSessions is the session tree: local shells and SSH sessions,
// each row with its own icon and state.
func (a *app) sidebarSessions(c *myui.Context) {
	t := c.Theme()
	myui.Scroll(c).Fill().Children(func() {
		myui.Column(c).FillWidth().Children(func() {
			local := a.sessionsOf(engine.Local)
			sectionHeader(c, "本地", len(local))
			myui.Column(c).FillWidth().Padding(0, 8, 6, 8).Gap(1).Children(func() {
				if len(local) == 0 && a.filter == "" {
					emptyNote(c, "尚无本地会话")
				}
				for _, s := range local {
					a.sessionRow(c, s)
				}
			})

			remote := a.sessionsOf(engine.SSH)
			sectionHeader(c, "SSH", len(remote))
			myui.Column(c).FillWidth().Padding(0, 8, 6, 8).Gap(1).Children(func() {
				if len(remote) == 0 && a.filter == "" {
					emptyNote(c, "尚无 SSH 会话")
				}
				for _, s := range remote {
					a.sessionRow(c, s)
				}
				add := myui.Row(c).FillWidth().Height(28).Padding(0, 10).Gap(7).
					Radius(6).AlignItems(myui.Center)
				if add.Hovered() {
					add = add.Background(t.SurfaceHover)
				}
				add.Children(func() {
					myui.Icon(c, icoPlus).FontSize(11).TextColor(t.Accent)
					myui.Text(c, "新建 SSH 连接").FontSize(11.5).TextColor(t.Accent)
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
func (a *app) sessionRow(c *myui.Context, s *engine.Session) {
	t := c.Theme()
	active := s.ID() == a.activeID
	exited := s.State() == engine.Exited

	row := myui.Row(c.Key(s.ID())).FillWidth().Height(30).Padding(0, 8).Gap(8).
		Radius(6).AlignItems(myui.Center)
	switch {
	case active:
		row = row.Background(t.SurfaceHover)
	case row.Hovered():
		row = row.Background(t.SurfaceHover.Alpha(0.55))
	}

	edge := myui.Transparent
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
		myui.Box(c).Width(2).Height(14).Radius(1).Background(edge).Shrink(0)
		icon := icoTerminal
		if s.Kind() == engine.SSH {
			icon = icoGlobe
		}
		myui.Icon(c, icon).FontSize(13).TextColor(iconColor)
		myui.Text(c, s.Title()).FontSize(12).FontWeight(weightOf(active, 500, 400)).
			TextColor(titleColor).Grow(1).SingleLine().Ellipsis("…")
		a.stateMark(c, s)
	})
	row.Tooltip(s.Title())
	row.PopoverMenu(func(m *myui.Menu) { a.sessionMenu(m, s) })
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
func (a *app) agentCard(c *myui.Context, n int, tool, target string) {
	t := c.Theme()
	myui.Column(c).FillWidth().Padding(0, 10, 8, 10).Children(func() {
		myui.Column(c).FillWidth().Padding(10).Gap(7).Radius(10).
			Background(t.Surface).Border(1, t.Warning.Alpha(0.28)).Children(func() {
			myui.Row(c).FillWidth().Gap(7).AlignItems(myui.Center).Children(func() {
				myui.Icon(c, icoBot).FontSize(13).TextColor(t.Warning)
				myui.Text(c, "AI Agent 操作中").FontSize(11).FontWeight(500).TextColor(t.Warning)
				myui.Spacer(c)
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
			myui.Row(c).FillWidth().Gap(8).AlignItems(myui.Center).Children(func() {
				myui.Text(c, "输入已锁定").FontSize(9.5).TextColor(t.TextMuted)
				myui.Spacer(c)
				myui.Text(c, "详情见分格").FontSize(9.5).TextColor(t.TextMuted)
			})
		})
	})
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
