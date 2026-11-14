// The terminal area: the split tree, the draggable dividers, the
// empty state and a pane's context menu.

package ui

import (
	"fmt"

	myui "github.com/egoist/mygo/ui"
)

// terminalArea shows the split panes as cards over the window's paper,
// with the find overlay above them when it is open.
func (a *app) terminalArea(c *myui.Context) {
	t := c.Theme()
	myui.Box(c).Fill().Background(t.Background).Children(func() {
		myui.Box(c).Fill().Padding(paneGap, paneGap, paneGap, paneGap).Children(func() {
			a.paneTree(c, a.splitRoot)
		})
		if a.findOpen {
			a.findOverlay(c)
		}
	})
}

// paneTree lays out a node of the split tree: a pane's card, or two
// subtrees with a divider between them that drags for the share of room.
func (a *app) paneTree(c *myui.Context, n *paneNode) {
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
	var box myui.Element
	if split.vertical {
		// side by side: first at left, second at right
		box = myui.Row(c.Key(spKey(n)))
	} else {
		// one above the other
		box = myui.Column(c.Key(spKey(n)))
	}
	box.Fill().MinWidth(0).MinHeight(0).AlignItems(myui.Stretch)
	bounds := box.Bounds()
	first, second, ratio := split.first, split.second, split.ratio
	box.Children(func() {
		myui.Box(c).Grow(ratio).Basis(0).MinWidth(0).MinHeight(0).Children(func() {
			a.paneTree(c, first)
		})
		a.paneDivider(c, split, bounds)
		myui.Box(c).Grow(1 - ratio).Basis(0).MinWidth(0).MinHeight(0).Children(func() {
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
func (a *app) paneDivider(c *myui.Context, split *splitNode, bounds myui.Rect) {
	t := c.Theme()
	div := myui.Box(c).Role(myui.RoleSplitter).Label("分格分隔线").Shrink(0)
	if split.vertical {
		div = div.Width(paneGap).Cursor(myui.CursorResizeColumn)
	} else {
		div = div.Height(paneGap).Cursor(myui.CursorResizeRow)
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
		div.Draw(func(p *myui.Painter, r myui.Rect) {
			if split.vertical {
				p.Fill(myui.Rect{X: r.X + r.W/2 - 1.5, Y: r.Y + r.H/2 - 18, W: 3, H: 36}, t.TextMuted.Alpha(0.55), 1.5)
			} else {
				p.Fill(myui.Rect{X: r.X + r.W/2 - 18, Y: r.Y + r.H/2 - 1.5, W: 36, H: 3}, t.TextMuted.Alpha(0.55), 1.5)
			}
		})
	}
}

// emptyPanes is the area with no session at all: the mark and the way to
// start one.
func (a *app) emptyPanes(c *myui.Context) {
	t := c.Theme()
	myui.Column(c).Fill().Center().Gap(12).Children(func() {
		myui.Icon(c, icoTerminal).FontSize(28).TextColor(t.TextMuted)
		myui.Text(c, "从左上角「新建会话」开始").FontSize(13).TextColor(t.Text)
		myui.Row(c).Gap(6).AlignItems(myui.Center).Children(func() {
			kbd(c, "Ctrl + Shift + T")
			myui.Text(c, "新建终端").FontSize(11.5).TextColor(t.TextMuted)
			myui.Text(c, "·").FontSize(11.5).TextColor(t.TextMuted)
			kbd(c, "Ctrl + F")
			myui.Text(c, "搜索").FontSize(11.5).TextColor(t.TextMuted)
		})
		myui.Text(c, "右键标签或终端可分屏").FontSize(11.5).TextColor(t.TextMuted)
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
func (a *app) paneMenu(m *myui.Menu, n *paneNode) {
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
