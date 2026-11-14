// The synchronized popout window: its own title strip over the
// session's mirror terminal.

package ui

import (
	"adakaki/internal/engine"

	"github.com/egoist/mygo/plugins/terminal"
	myui "github.com/egoist/mygo/ui"
)

// popoutView is a synchronized popout window: its own title strip over
// the session's mirror terminal.
func (a *app) popoutView(c *myui.Context, mv *engine.MirrorView, title string) {
	webTheme(c)
	c.SetMenuPresentation(myui.PopoverMenus)
	t := c.Theme()
	myui.Column(c).Fill().Background(t.Background).Children(func() {
		titleStrip(c, title+" — 同屏", nil)
		if term := mv.Terminal(); term != nil {
			terminal.View(c, term).Fill().AutoFocus()
			return
		}
		myui.Column(c).Fill().Center().Children(func() {
			myui.Text(c, "会话已结束").FontSize(12).TextColor(t.TextMuted)
		})
	})
}
