// The settings window's frame: its title strip, the category nav
// and the content pane, with the shared row helpers.

package ui

import (
	myui "github.com/egoist/mygo/ui"
)

// settingsView lays the window out: its own title strip, then nav |
// content.
func (a *app) settingsView(c *myui.Context) {
	webTheme(c)
	c.SetMenuPresentation(myui.PopoverMenus)
	t := c.Theme()
	myui.Column(c).Fill().Background(t.Background).Children(func() {
		titleStrip(c, "设置", func() {
			myui.Row(c).Gap(5).AlignItems(myui.Center).Children(func() {
				myui.Icon(c, icoCheck).FontSize(11).TextColor(t.Success)
				myui.Text(c, "改动即保存").FontSize(10).TextColor(t.TextMuted)
			})
		})
		myui.Split(c, &a.settingsNavW, func() { a.settingsNav(c) }, func() { a.settingsContent(c) }).Fill()
	})
}

// settingsGroup is a category of the nav.
type settingsGroup struct {
	title string
	items []settingsItem
}

type settingsItem struct {
	id    string
	icon  *myui.SVG
	label string
	desc  string
}

var settingsGroups = []settingsGroup{
	{
		title: "外观与终端",
		items: []settingsItem{
			{"外观", icoPalette, "外观", "终端配色方案；选中的方案立刻应用到全部终端。"},
			{"终端", icoTerminal, "终端", "字体家族与字号；改动即时生效，正在运行的终端一并跟随。"},
			{"会话", icoLayout, "会话", "新会话的默认 shell 与启动时的会话恢复。"},
		},
	},
	{
		title: "连接与自动化",
		items: []settingsItem{
			{"MCP", icoServer, "MCP 端点", "给 AI Agent 的 HTTP 端点：地址、端口与 token。"},
			{"安全", icoShield, "安全与护栏", "危险命令护栏、SSH 主机密钥校验与已信任主机。"},
		},
	},
}

// settingsNav is the category list on the left.
func (a *app) settingsNav(c *myui.Context) {
	t := c.Theme()
	myui.Column(c).Fill().Background(t.Surface).Children(func() {
		myui.Column(c).Padding(10, 10, 6, 10).Gap(2).FillWidth().Children(func() {
			for _, g := range settingsGroups {
				myui.Text(c, g.title).FontSize(9.5).FontWeight(500).LetterSpacing(0.5).
					TextColor(t.TextMuted).Padding(8, 8, 4, 8)
				for _, it := range g.items {
					a.settingsNavItem(c, it)
				}
			}
		})
		myui.Spacer(c)
		myui.Row(c).FillWidth().Height(34).Padding(0, 14).Gap(6).
			AlignItems(myui.Center).Children(func() {
			mono(c, "config.json · v0.2.0").FontSize(9).TextColor(t.TextMuted)
		})
	})
}

// settingsNavItem is one category: its glyph and name, tinted while it is
// the pane shown.
func (a *app) settingsNavItem(c *myui.Context, it settingsItem) {
	t := c.Theme()
	active := a.settingsCat == it.id
	row := myui.Row(c).FillWidth().Height(32).Padding(0, 8).Gap(9).
		Radius(6).AlignItems(myui.Center)
	switch {
	case active:
		row = row.Background(t.Background).Border(1, t.Border)
	case row.Hovered():
		row = row.Background(t.SurfaceHover.Alpha(0.6))
	}
	iconColor := t.TextMuted
	textColor := t.TextMuted
	weight := 400
	if active {
		iconColor = t.Accent
		textColor = t.Text
		weight = 500
	}
	row.Children(func() {
		myui.Icon(c, it.icon).FontSize(14).TextColor(iconColor)
		myui.Text(c, it.label).FontSize(12).FontWeight(weight).TextColor(textColor)
	})
	if row.Clicked() {
		a.settingsCat = it.id
	}
}

// settingsContent builds the category the nav chose.
func (a *app) settingsContent(c *myui.Context) {
	t := c.Theme()
	it := a.settingsItem(a.settingsCat)
	myui.Column(c).Fill().Background(t.Background).Children(func() {
		myui.Column(c).FillWidth().Padding(28, 28, 18, 28).Gap(6).Children(func() {
			myui.Text(c, it.label).FontSize(18).FontWeight(600)
			myui.Text(c, it.desc).FontSize(11.5).TextColor(t.TextMuted)
		})
		myui.Divider(c)
		myui.Scroll(c).Fill().Children(func() {
			myui.Column(c).Padding(24, 28).Gap(22).Children(func() {
				switch a.settingsCat {
				case "终端":
					a.terminalSettings(c)
				case "会话":
					a.sessionSettings(c)
				case "MCP":
					a.mcpSettings(c)
				case "安全":
					a.securitySettings(c)
				default:
					a.appearanceSettings(c)
				}
			})
		})
	})
}

// settingsItem finds a category's metadata.
func (a *app) settingsItem(id string) settingsItem {
	for _, g := range settingsGroups {
		for _, it := range g.items {
			if it.id == id {
				return it
			}
		}
	}
	return settingsItem{id: id, label: id}
}

// settingsSection is a titled block of rows.
func settingsSection(c *myui.Context, title string, fn func()) {
	myui.Column(c).FillWidth().Gap(10).Children(func() {
		sectionLabel(c, title)
		fn()
	})
}

// fieldRow is a label with a control beside it, the settings rows' shape.
func fieldRow(c *myui.Context, label string, control func()) {
	myui.Row(c).FillWidth().Gap(12).AlignItems(myui.Center).Children(func() {
		myui.Text(c, label).Width(140).FontSize(11.5).TextColor(c.Theme().TextMuted)
		control()
	})
}

// hint is the quiet line under a control.
func hint(c *myui.Context, text string) {
	myui.Text(c, text).FontSize(10.5).TextColor(c.Theme().TextMuted).Wrap()
}

// setAppearance forces the window's color scheme ("" follows the
// system), persists it and repaints every window.
func (a *app) setAppearance(mode string) {
	if a.store.Settings.Appearance == mode {
		return
	}
	a.store.Settings.Appearance = mode
	a.saveSettingsNow()
	a.applyAppearance()
	if a.settingsWin != nil {
		a.settingsWin.Invalidate()
	}
	if a.auditWin != nil {
		a.auditWin.Invalidate()
	}
	for _, w := range a.popouts {
		w.Invalidate()
	}
	a.refreshUI()
}
