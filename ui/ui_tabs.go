// The workspace tab strip: one tab per split tree, its title from
// the focused session, and the menus that close whole tabs.

package ui

import (
	"adakaki/internal/engine"
	"adakaki/internal/shells"
	"fmt"
	"path/filepath"
	"strings"

	myui "github.com/egoist/mygo/ui"
)

// tabBar is the strip of workspace tabs: several terminals in split
// panes make one tab, and a quick 新建 at its end.
func (a *app) tabBar(c *myui.Context) {
	t := c.Theme()
	a.saveActive()
	myui.Row(c).FillWidth().Height(tabBarHeight).Background(t.Surface).
		Padding(0, 8).Gap(2).AlignItems(myui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		if len(a.tabs) == 0 {
			myui.Text(c, "无标签页").FontSize(11.5).TextColor(t.TextMuted)
		}
		for _, ws := range a.tabs {
			a.workspaceTab(c, ws)
		}
		myui.Spacer(c)
		add := myui.Button(c, "").Label("新建会话").Tooltip("Ctrl+Shift+T").
			Padding(4).Radius(6).Background(myui.Transparent).BorderWidth(0).Shrink(0)
		if add.Hovered() {
			add = add.Background(t.SurfaceHover)
		}
		add.PopoverMenuButton(a.newSessionMenu).Children(func() {
			myui.Icon(c, icoPlus).FontSize(13).TextColor(t.TextMuted)
		})
		ghostButton(c, icoLayout, "向下分屏", "向下分屏", func() { a.splitFocused(false) })
		ghostButton(c, icoSearch, "终端搜索", "搜索终端（Ctrl+F）", a.openFind)
	})
}

// workspaceTab is one tab: a session's short title, its state dot and,
// when the tab holds several panes, their count; the close button closes
// every session in the tab.
func (a *app) workspaceTab(c *myui.Context, ws *workspace) {
	t := c.Theme()
	active := ws == a.activeTab
	s := a.representativeSession(ws)
	title := "新标签页"
	if s != nil {
		title = shortSessionTitle(s)
	}
	pill := myui.Row(c.Key(fmt.Sprintf("tab-%d", ws.id))).Height(tabPillHeight).
		Padding(0, 4, 0, 6).Gap(1).Radius(7, 7, 0, 0).
		AlignItems(myui.Center).AlignSelf(myui.End)
	switch {
	case active:
		pill = pill.Background(t.Background).Border(1, t.Border)
	case pill.Hovered():
		pill = pill.Background(t.SurfaceHover).Radius(7, 7, 0, 0)
	}
	hovered := pill.Hovered() || active

	pill.Children(func() {
		titleBtn := myui.Button(c, "").Label(title).Height(26).Padding(0, 6).
			Radius(5).Background(myui.Transparent).BorderWidth(0).Shrink(0)
		titleBtn.Children(func() {
			myui.Row(c).Gap(5).AlignItems(myui.Center).Children(func() {
				if s != nil {
					a.stateMark(c, s)
				}
				color := t.TextMuted
				if active {
					color = t.Text
				}
				myui.Text(c, title).FontSize(11.5).
					FontWeight(weightOf(active, 500, 400)).
					TextColor(color).MaxWidth(96).SingleLine().Ellipsis("…")
				if n := leafCount(ws.root); n > 1 {
					myui.Text(c, fmt.Sprintf("×%d", n)).FontSize(9.5).
						TextColor(t.TextMuted.Alpha(0.8))
				}
			})
		})
		titleBtn.PopoverMenu(func(m *myui.Menu) { a.tabMenu(m, ws) })
		if titleBtn.Clicked() {
			a.activateTab(ws)
		}

		closeBtn := myui.Button(c, "").Label("关闭标签页").Padding(3).Radius(5).
			Background(myui.Transparent).BorderWidth(0).Shrink(0)
		if closeBtn.Hovered() {
			closeBtn = closeBtn.Background(t.SurfaceHover)
		}
		closeBtn.Children(func() {
			color := t.TextMuted
			if !hovered {
				color = myui.Transparent
			}
			myui.Icon(c, icoX).FontSize(11).TextColor(color)
		})
		if closeBtn.Clicked() {
			a.closeTab(ws)
		}
	})
	pill.PopoverMenu(func(m *myui.Menu) { a.tabMenu(m, ws) })
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
func (a *app) tabMenu(m *myui.Menu, ws *workspace) {
	if m.Item("新建标签页").Shortcut(myui.Cmd|myui.Shift, myui.KeyT).Chosen() {
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
