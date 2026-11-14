package ui

// The notification area icon: a quick way back to the console, a new
// session, and a quit that asks first, as the window's close button does.
//
// Its menu is drawn by the app, not by the system: a flyout window
// anchored to the icon, in the same surface, accent and type as the rest
// of the console. Linux's AppIndicator shows no clicks and no icon
// bounds, so it keeps a native menu there.

import (
	"fmt"
	"log"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	myui "github.com/egoist/mygo/ui"
)

// setupTray adds the app's icon to the notification area (Windows) or the
// menu bar (macOS). It must run after the application is ready.
func (a *app) setupTray() {
	icon := trayIconPNG
	template := false
	if runtime.GOOS == "darwin" {
		// A black-on-transparent glyph lets macOS tint it for the menu bar.
		icon, template = trayTemplatePNG, true
	}
	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID:             "adakaki",
		Icon:           icon,
		IconIsTemplate: template,
		ToolTip:        "AdaKaki 控制台",
	})
	if err != nil {
		// The console keeps working without a notification area entry.
		log.Printf("tray: %v", err)
		return
	}
	a.tray = tray

	if runtime.GOOS == "linux" {
		tray.SetMenu(a.trayNativeMenu())
	} else {
		// Make the menu now, hidden, so the right click only shows it.
		a.createTrayMenu()
		tray.OnClick(a.showMainWindow)
		tray.OnRightClick(a.toggleTrayMenu)
	}
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		a.closeTrayMenu()
		tray.Destroy()
	})
}

// trayNativeMenu is the fallback for Linux, whose panel shows only menus,
// and tells the app neither clicks nor where its icon is.
func (a *app) trayNativeMenu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: "打开主窗口", Click: func(*mygo.MenuItem, *mygo.Window) {
			a.showMainWindow()
		}},
		{Label: "新建会话", Click: func(*mygo.MenuItem, *mygo.Window) {
			a.showMainWindow()
			a.newSession(a.defaultShellID(), "", "")
		}},
		mygo.Separator(),
		{Label: "退出 AdaKaki", Click: func(*mygo.MenuItem, *mygo.Window) {
			a.requestQuit()
		}},
	})
}

// createTrayMenu builds the drawn menu once, hidden: making a window and
// laying out its first frame inside the right-click handler is what made
// the old menu slow to appear.
func (a *app) createTrayMenu() {
	menu := mygo.NewFlyout(mygo.FlyoutOptions{
		Tray:         a.tray,
		Placement:    mygo.PlacementBottom,
		Gap:          14,
		Width:        236,
		Height:       168,
		FitContent:   true,
		MinWidth:     236,
		MaxWidth:     236,
		MaxHeight:    320,
		Hidden:       true,
		Focusable:    true,
		Shadow:       true,
		CornerRadius: 10,
		Content:      myui.View(a.trayMenuView),
	})
	menu.OnClose(func(e *mygo.CloseEvent) {
		// Keep the window for the next click; App.Quit still closes it.
		if a.quitting {
			return
		}
		e.PreventDefault()
		menu.Hide()
	})
	menu.OnHide(func() { a.trayClosed = time.Now() })
	a.trayMenu = menu
}

// toggleTrayMenu opens the drawn menu beside the icon, or hides it when
// it already shows.
func (a *app) toggleTrayMenu() {
	menu := a.trayMenu
	if menu == nil {
		return
	}
	if menu.IsVisible() {
		a.closeTrayMenu()
		return
	}
	// A right click on the icon takes the keyboard from the open menu,
	// which hides before this callback: without a beat, the very click
	// that closed the menu would open it again.
	if !a.trayClosed.IsZero() && time.Since(a.trayClosed) < 250*time.Millisecond {
		return
	}
	menu.Show()
}

// closeTrayMenu dismisses the drawn menu, if it shows.
func (a *app) closeTrayMenu() {
	if a.trayMenu == nil || !a.trayMenu.IsVisible() {
		return
	}
	a.trayMenu.Hide()
}

// trayMenuView is the drawn menu: the app's identity over the three
// actions, with the console's own icons and hover.
func (a *app) trayMenuView(c *myui.Context) {
	webTheme(c)
	// The flyout itself is transparent: no opaque window fill may show
	// through the rounded corners.
	c.Root().Background(myui.Transparent)
	t := c.Theme()
	root := myui.Column(c).Fill()
	if c.Vibrancy() {
		// the system's material shows through
	} else {
		// The flyout window is transparent: the rounded surface is the
		// menu's own shape, the same on every platform.
		root = root.Background(t.Surface).Radius(10).Border(1, t.Border)
	}
	root.Padding(6).Children(func() {
		myui.Row(c).FillWidth().Height(38).Padding(0, 8).Gap(9).
			AlignItems(myui.Center).Children(func() {
			a.logoMark(c)
			myui.Column(c).Gap(1).Children(func() {
				myui.Text(c, "AdaKaki 控制台").FontSize(12.5).FontWeight(600)
				myui.Text(c, "桌面多路终端").FontSize(9.5).TextColor(t.TextMuted)
			})
			myui.Spacer(c)
			if n := a.mgr.Count(); n > 0 {
				chip(c, fmt.Sprintf("%d 个会话", n), t.TextMuted, t.Background)
			}
		})
		trayMenuDivider(c)

		trayMenuRow(c, icoAppWindow, "打开主窗口", "", false, func() {
			a.closeTrayMenu()
			a.showMainWindow()
		})
		trayMenuRow(c, icoPlus, "新建会话", "Ctrl + Shift + T", false, func() {
			a.closeTrayMenu()
			a.showMainWindow()
			a.newSession(a.defaultShellID(), "", "")
		})
		trayMenuDivider(c)
		trayMenuRow(c, icoPower, "退出 AdaKaki", "", true, func() {
			a.closeTrayMenu()
			a.requestQuit()
		})
	})
}

// trayMenuRow is one action of the drawn menu: an icon, a label, the fast
// path's keys, and a hover tint.
func trayMenuRow(c *myui.Context, svg *myui.SVG, label, hint string, danger bool, fn func()) {
	t := c.Theme()
	color := t.Text
	iconColor := t.TextMuted
	if danger {
		color, iconColor = t.Danger, t.Danger
	}
	b := myui.Button(c, "").Label(label).FillWidth().Height(34).Padding(0, 8).
		Radius(7).Background(myui.Transparent).BorderWidth(0).Shrink(0)
	if b.Hovered() {
		b = b.Background(t.SurfaceHover)
	}
	b.Children(func() {
		myui.Icon(c, svg).FontSize(14).TextColor(iconColor)
		myui.Text(c, label).FontSize(12).FontWeight(500).TextColor(color)
		myui.Spacer(c)
		if hint != "" {
			kbd(c, hint)
		}
	})
	if b.Clicked() {
		fn()
	}
}

// trayMenuDivider separates the menu's groups.
func trayMenuDivider(c *myui.Context) {
	myui.Box(c).FillWidth().Height(1).Margin(4, 3).
		Background(c.Theme().Border)
}

// requestQuit follows the window's close path: with sessions still
// running it opens the console and asks, rather than pulling the sessions
// out from under the user.
func (a *app) requestQuit() {
	if a.win != nil && a.mgr.Count() > 0 && !a.quitArmed {
		a.showMainWindow()
		a.win.Update(func() { a.showQuit = true })
		return
	}
	a.quit()
}

// showMainWindow brings the console back from hidden or minimized, and
// focuses it.
func (a *app) showMainWindow() {
	if a.win == nil {
		return
	}
	if a.win.IsMinimized() {
		a.win.Restore()
	}
	a.win.Show()
	a.win.Focus()
}
