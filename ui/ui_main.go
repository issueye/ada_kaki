// The main window's frame: the shared header, the sidebar or rail,
// the tabbed content and the footer, with the dialogs above them.

package ui

import (
	myui "github.com/egoist/mygo/ui"
)

// mainView builds the main window: a sidebar that runs to the top of the
// window beside the content pane — the title strip with the actions, the
// tab bar, the terminal panes and the status bar — with the dialogs over
// them.
func (a *app) mainView(c *myui.Context) {
	webTheme(c)
	c.SetMenuPresentation(myui.PopoverMenus)
	a.saveActive() // the tab bar and body show the active workspace
	if a.pendingToast != "" {
		c.Toast(a.pendingToast)
		a.pendingToast = ""
	}

	// ---- keyboard shortcuts (window level, ahead of the terminals) ----
	if c.Shortcut(myui.Cmd|myui.Shift, myui.KeyT) {
		a.newSession(a.defaultShellID(), "", "")
	}
	if c.Shortcut(myui.Cmd, myui.KeyTab) {
		a.cycleTab(1)
	}
	if c.Shortcut(myui.Cmd|myui.Shift, myui.KeyTab) {
		a.cycleTab(-1)
	}
	if c.Shortcut(myui.Cmd|myui.Shift, myui.KeyW) {
		a.closeTab(a.activeTab)
	}
	if c.Shortcut(myui.Cmd|myui.Shift, myui.KeyF12) {
		if s := a.active(); s != nil {
			a.forceCloseSession(s)
		}
	}
	if c.Shortcut(myui.Cmd|myui.Shift, myui.KeyP) {
		if s := a.active(); s != nil {
			a.popout(s)
		}
	}
	if c.Shortcut(myui.Cmd, myui.KeyF) {
		a.openFind()
	}
	if c.Shortcut(myui.Cmd, myui.KeyB) {
		a.sidebarCollapsed = !a.sidebarCollapsed
	}
	if c.Shortcut(myui.Cmd|myui.Alt, myui.KeyA) {
		a.toggleAIDock()
	}

	myui.Column(c).Fill().Children(func() {
		a.appHeader(c)
		// The body alone collapses: the header and footer run across the
		// whole window, over the sidebar and the content alike.
		myui.Box(c).FillWidth().Grow(1).Children(func() {
			if a.sidebarCollapsed {
				myui.Row(c).Fill().Children(func() {
					a.sidebarRail(c)
					myui.Column(c).Grow(1).FillHeight().Children(func() { a.contentPane(c) })
				})
			} else {
				// The drag writes the width; keep it within what the rows
				// can still lay out in step.
				a.sidebarWidth = clampSidebarWidth(a.sidebarWidth)
				myui.Split(c, &a.sidebarWidth,
					func() { a.sidebarPane(c) },
					func() { a.contentPane(c) }).Fill()
			}
		})
		a.appFooter(c)
	})

	a.mainDialogs(c)
}

// contentPane is the right column below the shared header: the tab bar
// and the terminal panes, with the AI dock beside them when it is open.
// The split is laid out right to left so that the dock, its first pane,
// sits on the right and the divider drags its width.
func (a *app) contentPane(c *myui.Context) {
	content := func() {
		myui.Column(c).Fill().Direction(myui.LeftToRight).
			Background(c.Theme().Background).Children(func() {
			a.tabBar(c)
			a.terminalArea(c)
		})
	}
	if !a.aiDockVisible() {
		content()
		return
	}
	a.aiDockW = clampAIDockWidth(a.aiDockW)
	sp := myui.Split(c, &a.aiDockW, func() { a.aiDock(c) }, content).Fill()
	sp.Direction(myui.RightToLeft)
	// The divider writes the width every frame it moves; persist once the
	// drag has settled instead of on each frame.
	if sp.Changed() {
		a.aiDockW = clampAIDockWidth(a.aiDockW)
	} else if int(a.aiDockW) != a.aiDockSaved {
		a.aiDockSaved = int(a.aiDockW)
		a.store.Settings.AI.DockWidth = a.aiDockSaved
		a.saveSettingsNow()
	}
}
