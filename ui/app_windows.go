// The secondary windows and the settings they apply: audit and
// settings windows, appearance, fonts and terminal themes.

package ui

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	myui "github.com/egoist/mygo/ui"
)

// openAudit shows the audit trail viewer in its own window.
func (a *app) openAudit() {
	if a.win == nil || a.auditWin != nil {
		return
	}
	a.auditWin = mygo.NewWindow(mygo.WindowOptions{
		Title:         "MCP 审计日志",
		TitleBarStyle: mygo.TitleBarHidden,
		Width:         1100,
		Height:        720,
		Content: myui.View(func(c *myui.Context) {
			a.auditView(c)
		}),
	})
	iconWindow(a.auditWin)
	a.auditWin.OnClosed(func() { a.auditWin = nil })
}

// openSettings opens the settings window: a nav of categories on the
// left, the category's content on the right. Changes apply live and save
// as they are made.
func (a *app) openSettings() {
	if a.win == nil || a.settingsWin != nil {
		return
	}
	a.settingsCat = "外观"
	a.settingsWin = mygo.NewWindow(mygo.WindowOptions{
		Title:         "设置",
		TitleBarStyle: mygo.TitleBarHidden,
		Width:         1040,
		Height:        720,
		Content: myui.View(func(c *myui.Context) {
			a.settingsView(c)
		}),
	})
	iconWindow(a.settingsWin)
	a.settingsWin.OnClosed(func() { a.settingsWin = nil })
}

// saveSettingsNow persists the configuration; the settings page calls it
// after every change.
func (a *app) saveSettingsNow() {
	if err := a.store.Save(); err != nil {
		a.pendingToast = "保存设置失败：" + err.Error()
		a.refreshUI()
	}
}

// applyFont pushes the configured font to the engine: new sessions start
// with it, running ones switch live.
func (a *app) applyFont() {
	a.mgr.SetFont(terminal.Font{
		Family: a.store.Settings.FontFamily,
		Size:   float32(a.store.Settings.FontSize),
	})
}

// applyTheme pushes the configured color scheme to the engine; "" means
// follow the window's appearance.
func (a *app) applyTheme() {
	if err := a.mgr.SetTheme(a.store.Settings.TerminalTheme); err != nil {
		a.pendingToast = "配色方案 " + a.store.Settings.TerminalTheme + " 不存在"
		a.refreshUI()
	}
}

// appearanceSource maps the setting to the framework's theme source.
func (a *app) appearanceSource() mygo.ThemeSource {
	switch a.store.Settings.Appearance {
	case "dark":
		return mygo.ThemeDark
	case "light":
		return mygo.ThemeLight
	}
	return mygo.ThemeSystem
}

// applyAppearance tells the framework the app's appearance: the palette,
// the window controls of hidden title bars, the menus and the terminals'
// light or dark defaults all follow it.
func (a *app) applyAppearance() {
	mygo.Theme.SetSource(a.appearanceSource())
}
