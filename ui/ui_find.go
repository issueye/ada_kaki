// The find bar over the terminal area.

package ui

import (
	myui "github.com/egoist/mygo/ui"
)

// findOverlay is the terminal search: a floating card in the pane's top
// right, so the terminal keeps its height.
func (a *app) findOverlay(c *myui.Context) {
	t := c.Theme()
	myui.Row(c).Absolute().Top(10).Right(24).Width(420).Height(40).Padding(0, 8).
		Gap(8).Radius(8).Background(t.Surface).Border(1, t.Border).
		Shadow(0, 12, 28, -6, myui.RGBA(0, 0, 0, 0.28)).
		AlignItems(myui.Center).Children(func() {
		myui.Icon(c, icoSearch).FontSize(13).TextColor(t.TextMuted)
		myui.TextInput(c.Key("find-input"), &a.findText).Grow(1).Height(28).
			FontSize(12).BorderWidth(0).Background(myui.Transparent).
			Placeholder("搜索终端").PlaceholderColor(t.TextMuted)
		if a.findText != "" {
			myui.Text(c, a.findStatus()).Font(monoFamily()).FontSize(10.5).
				TextColor(t.TextMuted)
		}
		buttons := []struct {
			icon       *myui.SVG
			label, tip string
			fn         func()
		}{
			{icoChevronUp, "上一个匹配", "上一个（Shift+Enter）", a.searchPrev},
			{icoChevronDown, "下一个匹配", "下一个（Enter）", a.searchNext},
			{icoX, "关闭搜索", "关闭（Esc）", a.closeFind},
		}
		for _, b := range buttons {
			btn := myui.Button(c, "").Label(b.label).Padding(3).Radius(5).
				Background(myui.Transparent).BorderWidth(0).Shrink(0)
			if btn.Hovered() {
				btn = btn.Background(t.SurfaceHover)
			}
			btn.Tooltip(b.tip).Children(func() {
				myui.Icon(c, b.icon).FontSize(12).TextColor(t.TextMuted)
			})
			if btn.Clicked() {
				b.fn()
			}
		}
	})
}
