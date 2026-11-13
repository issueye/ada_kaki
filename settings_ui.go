package main

// The settings window: a nav of categories on the left, the category's
// content on the right. Every change applies live — the engine takes it
// at once — and saves to the config as it is made.

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// settingsView lays the window out: its own title strip, then nav |
// content.
func (a *app) settingsView(c *ui.Context) {
	webTheme(c)
	c.SetMenuPresentation(ui.PopoverMenus)
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		titleStrip(c, "设置", func() {
			ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, icoCheck).FontSize(11).TextColor(t.Success)
				ui.Text(c, "改动即保存").FontSize(10).TextColor(t.TextMuted)
			})
		})
		ui.Split(c, &a.settingsNavW, func() { a.settingsNav(c) }, func() { a.settingsContent(c) }).Fill()
	})
}

// settingsGroup is a category of the nav.
type settingsGroup struct {
	title string
	items []settingsItem
}

type settingsItem struct {
	id    string
	icon  *ui.SVG
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
func (a *app) settingsNav(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Surface).Children(func() {
		ui.Column(c).Padding(10, 10, 6, 10).Gap(2).FillWidth().Children(func() {
			for _, g := range settingsGroups {
				ui.Text(c, g.title).FontSize(9.5).FontWeight(500).LetterSpacing(0.5).
					TextColor(t.TextMuted).Padding(8, 8, 4, 8)
				for _, it := range g.items {
					a.settingsNavItem(c, it)
				}
			}
		})
		ui.Spacer(c)
		ui.Row(c).FillWidth().Height(34).Padding(0, 14).Gap(6).
			AlignItems(ui.Center).Children(func() {
			mono(c, "config.json · v0.2.0").FontSize(9).TextColor(t.TextMuted)
		})
	})
}

// settingsNavItem is one category: its glyph and name, tinted while it is
// the pane shown.
func (a *app) settingsNavItem(c *ui.Context, it settingsItem) {
	t := c.Theme()
	active := a.settingsCat == it.id
	row := ui.Row(c).FillWidth().Height(32).Padding(0, 8).Gap(9).
		Radius(6).AlignItems(ui.Center)
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
		ui.Icon(c, it.icon).FontSize(14).TextColor(iconColor)
		ui.Text(c, it.label).FontSize(12).FontWeight(weight).TextColor(textColor)
	})
	if row.Clicked() {
		a.settingsCat = it.id
	}
}

// settingsContent builds the category the nav chose.
func (a *app) settingsContent(c *ui.Context) {
	t := c.Theme()
	it := a.settingsItem(a.settingsCat)
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		ui.Column(c).FillWidth().Padding(28, 28, 18, 28).Gap(6).Children(func() {
			ui.Text(c, it.label).FontSize(18).FontWeight(600)
			ui.Text(c, it.desc).FontSize(11.5).TextColor(t.TextMuted)
		})
		ui.Divider(c)
		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).Padding(24, 28).Gap(22).Children(func() {
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
func settingsSection(c *ui.Context, title string, fn func()) {
	ui.Column(c).FillWidth().Gap(10).Children(func() {
		sectionLabel(c, title)
		fn()
	})
}

// fieldRow is a label with a control beside it, the settings rows' shape.
func fieldRow(c *ui.Context, label string, control func()) {
	ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).Width(140).FontSize(11.5).TextColor(c.Theme().TextMuted)
		control()
	})
}

// hint is the quiet line under a control.
func hint(c *ui.Context, text string) {
	ui.Text(c, text).FontSize(10.5).TextColor(c.Theme().TextMuted).Wrap()
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

// ---- 外观 ----

func (a *app) appearanceSettings(c *ui.Context) {
	t := c.Theme()
	current := a.store.Settings.TerminalTheme
	label := current
	if label == "" {
		label = "跟随窗口（浅色/深色自动切换）"
	}
	settingsSection(c, "界面主题", func() {
		seg := ui.Row(c).Width(312).Height(34).Padding(3).Gap(2).Radius(8).
			Background(t.Surface).Border(1, t.Border)
		seg.Children(func() {
			options := []struct{ id, label string }{
				{"", "跟随系统"}, {"light", "浅色"}, {"dark", "深色"},
			}
			for _, o := range options {
				active := a.store.Settings.Appearance == o.id
				b := ui.Button(c, o.label).Height(28).Grow(1).Radius(6).FontSize(11)
				if active {
					b = b.Background(t.SurfacePressed).FontWeight(500)
				} else {
					b = b.Background(ui.Transparent)
				}
				if b.Clicked() {
					a.setAppearance(o.id)
				}
			}
		})
		hint(c, "强制浅色或深色会同时切换终端配色；选了具体配色方案时以方案为准。")
	})

	settingsSection(c, "终端配色方案", func() {
		ui.Row(c).FillWidth().Padding(12).Gap(12).Radius(8).
			Background(t.Surface).Border(1, t.Border).
			AlignItems(ui.Center).Children(func() {
			a.themeSwatch(c, t)
			ui.Column(c).Gap(3).Grow(1).Children(func() {
				ui.Text(c, label).FontSize(12.5).FontWeight(500)
				ui.Text(c, fmt.Sprintf("Ghostty 主题目录 · %d 个方案", len(a.ghosttyThemes))).
					FontSize(10.5).TextColor(t.TextMuted)
			})
			if current != "" {
				if ui.Button(c, "跟随窗口").Height(28).Padding(0, 10).Radius(6).
					FontSize(11).Clicked() {
					a.store.Settings.TerminalTheme = ""
					a.applyTheme()
					a.saveSettingsNow()
				}
			} else {
				chip(c, "跟随系统", t.TextMuted, t.Background)
			}
		})
		hint(c, "选择后立即应用到全部运行中的终端，新会话同样使用。来自 Ghostty 的配色目录。")
	})

	settingsSection(c, "搜索方案", func() {
		ui.Row(c).FillWidth().Height(32).Padding(0, 10).Gap(7).Radius(7).
			Background(t.Background).Border(1, t.Border).
			AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icoSearch).FontSize(12).TextColor(t.TextMuted)
			ui.TextInput(c.Key("theme-filter"), &a.themeFilter).Grow(1).Height(26).
				FontSize(11.5).BorderWidth(0).Background(ui.Transparent).
				Placeholder("输入名称过滤，如 catppuccin / dracula").
				PlaceholderColor(t.TextMuted).AutoFocus()
			if a.themeFilter != "" {
				if ui.Button(c, "").Label("清除").Padding(2).Radius(4).
					Background(ui.Transparent).BorderWidth(0).Children(func() {
					ui.Icon(c, icoX).FontSize(11).TextColor(t.TextMuted)
				}).Clicked() {
					a.themeFilter = ""
				}
			}
		})
		names := a.filteredThemes()
		ui.Text(c, fmt.Sprintf("%d 个方案", len(names))).FontSize(10).TextColor(t.TextMuted)
		ui.Box(c).FillWidth().Height(300).Radius(8).Background(t.Background).
			Border(1, t.Border).Children(func() {
			ui.Scroll(c).Fill().Padding(4).Children(func() {
				ui.Column(c).FillWidth().Gap(1).Children(func() {
					shown := 0
					for _, name := range names {
						if shown >= 60 {
							ui.Text(c, "…继续输入名称以缩小范围").FontSize(10.5).
								TextColor(t.TextMuted).Padding(6, 10)
							return
						}
						shown++
						a.themeRow(c, name)
					}
				})
			})
		})
		hint(c, "列表按名称过滤，选中一行即应用；「跟随窗口」随系统深浅色切换。")
	})
}

// themeSwatch is the four-color sample beside the current scheme's name.
func (a *app) themeSwatch(c *ui.Context, t *ui.Theme) {
	colors := []ui.Color{t.Surface, t.Accent, t.Success, t.Warning}
	ui.Row(c).Width(44).Height(30).Gap(3).AlignItems(ui.Center).Shrink(0).Children(func() {
		for _, col := range colors {
			ui.Box(c).Width(9).Height(24).Radius(3).Background(col)
		}
	})
}

// themeRow is one scheme in the catalog.
func (a *app) themeRow(c *ui.Context, name string) {
	t := c.Theme()
	selected := a.store.Settings.TerminalTheme == name
	row := ui.Row(c).FillWidth().Height(30).Padding(0, 8).Gap(8).
		Radius(5).AlignItems(ui.Center)
	switch {
	case selected:
		row = row.Background(t.Accent.Alpha(0.12))
	case row.Hovered():
		row = row.Background(t.Surface)
	}
	row.Children(func() {
		if selected {
			ui.Icon(c, icoCheck).FontSize(11).TextColor(t.Accent)
		} else {
			ui.Box(c).Width(11)
		}
		color := t.Text
		if selected {
			color = t.Accent
		}
		mono(c, name).FontSize(11).TextColor(color).Grow(1).
			SingleLine().Ellipsis("…")
	})
	if row.Clicked() {
		a.store.Settings.TerminalTheme = name
		a.applyTheme()
		a.saveSettingsNow()
	}
}

func (a *app) filteredThemes() []string {
	filter := strings.ToLower(strings.TrimSpace(a.themeFilter))
	out := make([]string, 0, len(a.ghosttyThemes))
	for _, n := range a.ghosttyThemes {
		if filter == "" || strings.Contains(strings.ToLower(n), filter) {
			out = append(out, n)
		}
	}
	return out
}

// ---- 终端 ----

func (a *app) terminalSettings(c *ui.Context) {
	t := c.Theme()
	settingsSection(c, "字体", func() {
		fieldRow(c, "字体", func() {
			a.fontPicker(c)
		})
		fieldRow(c, "自定义", func() {
			in := ui.TextInput(c.Key("set-font-family"), &a.store.Settings.FontFamily).
				Placeholder("逗号分隔候选，如 Cascadia Code, Consolas").Grow(1)
			if in.Changed() || in.Submitted() {
				a.applyFont()
				a.saveSettingsNow()
			}
		})
		hint(c, "列表里是系统等宽字体与内置字体；「自定义」可写回退列表，留空用系统等宽。")
		fieldRow(c, "字号（DIP）", func() {
			in := ui.TextInput(c.Key("set-font-size"), &a.sizeText).
				Placeholder("默认 13").Width(80)
			if in.Changed() || in.Submitted() {
				var n int
				fmt.Sscanf(a.sizeText, "%d", &n)
				if n == 0 || (n >= 6 && n <= 40) {
					a.store.Settings.FontSize = n
					a.applyFont()
					a.saveSettingsNow()
				}
			}
		})
		hint(c, "改动立即应用到全部运行中的终端。")
	})

	settingsSection(c, "当前字体", func() {
		family := firstFamily(a.store.Settings.FontFamily)
		if family == "" {
			family = "系统等宽字体"
		}
		size := a.store.Settings.FontSize
		if size == 0 {
			size = 13
		}
		ui.Row(c).FillWidth().Padding(14).Radius(8).Background(t.Surface).
			Border(1, t.Border).Children(func() {
			ui.Text(c, "PS> go test ./...").Font(family).FontSize(float32(size) + 1).
				TextColor(t.Text)
		})
		hint(c, fmt.Sprintf("预览：%s · %d DIP", family, size))
	})
}

// fontPicker is the font family list: a menu button whose label is the
// current family, the bundled fonts marked, the system's after them.
func (a *app) fontPicker(c *ui.Context) {
	t := c.Theme()
	pick := ui.Button(c, "").Label(a.fontLabel()).Height(30).Padding(0, 10).
		Radius(6).Grow(1).Background(t.Surface).Border(1, t.Border)
	if pick.Hovered() {
		pick = pick.Background(t.SurfaceHover)
	}
	pick.PopoverMenuButton(func(m *ui.Menu) {
		if m.Item("系统等宽字体").Checked(a.store.Settings.FontFamily == "").Chosen() {
			a.setFontFamily("")
		}
		m.Separator()
		for _, f := range a.monoFamilies() {
			label := f.Name
			if f.Registered {
				label += "（内置）"
			}
			if m.Item(label).Checked(a.store.Settings.FontFamily == f.Name).Chosen() {
				a.setFontFamily(f.Name)
			}
		}
	})
	pick.Children(func() {
		ui.Text(c, a.fontLabel()).FontSize(11.5).TextColor(t.Text).Grow(1).
			SingleLine().Ellipsis("…")
		ui.Icon(c, icoChevronDown).FontSize(11).TextColor(t.TextMuted)
	})
}

// fontLabel names the current font setting.
func (a *app) fontLabel() string {
	if strings.TrimSpace(a.store.Settings.FontFamily) == "" {
		return "系统等宽字体"
	}
	return a.store.Settings.FontFamily
}

// setFontFamily applies and saves a font family chosen from the list.
func (a *app) setFontFamily(name string) {
	a.store.Settings.FontFamily = name
	a.applyFont()
	a.saveSettingsNow()
	if a.settingsWin != nil {
		a.settingsWin.Invalidate()
	}
	a.refreshUI()
}

// monoFamilies caches the monospaced families the font setting offers:
// the app's registered fonts first, then the system's.
func (a *app) monoFamilies() []ui.FontFamily {
	if a.fontFamilies == nil {
		for _, f := range ui.FontFamilies() {
			if f.Monospaced {
				a.fontFamilies = append(a.fontFamilies, f)
			}
		}
	}
	return a.fontFamilies
}

// ---- 会话 ----

func (a *app) sessionSettings(c *ui.Context) {
	settingsSection(c, "新会话", func() {
		fieldRow(c, "默认 Shell", func() {
			ui.PopoverMenuButton(c, a.defaultShellLabel(), func(m *ui.Menu) {
				for _, sh := range a.shells {
					if sh.Found {
						if m.Item(sh.Name).Checked(a.store.Settings.DefaultShell == string(sh.ID)).Chosen() {
							a.store.Settings.DefaultShell = string(sh.ID)
							a.saveSettingsNow()
						}
					}
				}
			}).Grow(1)
		})
		hint(c, "快捷键 Ctrl+Shift+T、分屏的新分格、启动恢复都用它。")
	})

	settingsSection(c, "启动", func() {
		if ui.Checkbox(c, &a.store.Settings.RestoreDisabled, "启动时不恢复上次的会话").
			Changed() {
			a.saveSettingsNow()
		}
		hint(c, "关闭后，重启时按上次的标签与分格原样恢复；SSH 档案会话会自动重拨。")
	})
}

// ---- MCP ----

func (a *app) mcpSettings(c *ui.Context) {
	t := c.Theme()
	port := a.store.Settings.MCPHTTPPort
	if port == 0 {
		port = 8731
	}
	settingsSection(c, "HTTP 端点", func() {
		if ui.Checkbox(c, &a.store.Settings.MCPHTTP, "启动 MCP HTTP 端点（仅回环地址）").
			Changed() {
			a.saveSettingsNow()
		}
		if a.store.Settings.MCPHTTP {
			fieldRow(c, "端口", func() {
				in := ui.TextInput(c.Key("set-mcp-port"), &a.setMCPPort).
					Placeholder("8731").Width(120)
				if in.Changed() || in.Submitted() {
					var p int
					fmt.Sscanf(a.setMCPPort, "%d", &p)
					if p > 0 && p < 65536 {
						a.store.Settings.MCPHTTPPort = p
						a.saveSettingsNow()
					}
				}
			})
			ui.Row(c).FillWidth().Padding(12).Radius(8).Background(t.Surface).
				Border(1, t.Border).AlignItems(ui.Center).Gap(8).Children(func() {
				dot(c, t.Success, 6)
				mono(c, fmt.Sprintf("http://127.0.0.1:%d/mcp", port)).
					FontSize(11).TextColor(t.Text).Grow(1)
				if ui.Button(c, "复制地址").Height(26).Padding(0, 9).Radius(6).
					FontSize(10.5).Clicked() {
					mygo.Clipboard.WriteText(fmt.Sprintf("http://127.0.0.1:%d/mcp", port))
					a.pendingToast = "MCP 地址已复制"
					a.refreshUI()
				}
			})
			hint(c, "Token 在配置目录的 token 文件中；端口修改重启后生效。仅监听 127.0.0.1。")
		}
	})

	settingsSection(c, "stdio", func() {
		ui.Text(c, "命令行 adakaki mcp 始终可用：AI 客户端以子进程方式驱动，无 GUI 时也能创建会话。").
			FontSize(11).TextColor(t.TextMuted).Wrap()
	})
}

// ---- 安全 ----

func (a *app) securitySettings(c *ui.Context) {
	t := c.Theme()
	settingsSection(c, "命令护栏", func() {
		if ui.Checkbox(c, &a.store.Settings.GuardDisabled,
			"关闭危险命令护栏（rm -rf 等不再确认/拦截）").Changed() {
			a.guard.Enabled = !a.store.Settings.GuardDisabled
			a.saveSettingsNow()
		}
		if !a.store.Settings.GuardDisabled {
			builtin, custom := a.guard.Count()
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				chip(c, fmt.Sprintf("内置 %d 条", builtin), t.TextMuted, t.Surface)
				chip(c, fmt.Sprintf("自定义 %d 条", custom), t.TextMuted, t.Surface)
			})
			hint(c, "拦截与放行均记录于审计。自定义规则写在 config.json 的 guard_deny / guard_confirm。")
		}
	})

	settingsSection(c, "SSH 主机密钥", func() {
		fieldRow(c, "未知主机", func() {
			ui.PopoverMenuButton(c, strictLabel(a.store.Settings.StrictHostKey), func(m *ui.Menu) {
				for _, v := range []struct{ val, label string }{
					{"prompt", "首次连接弹窗确认"},
					{"accept_new", "自动信任新主机"},
					{"off", "关闭校验（不安全）"},
				} {
					if m.Item(v.label).Chosen() {
						a.store.Settings.StrictHostKey = v.val
						a.saveSettingsNow()
					}
				}
			}).Grow(1)
		})
		hint(c, "已知主机的指纹一旦变化，连接一律拒绝（防中间人）。")

		hosts := a.hostKeys.List()
		if len(hosts) == 0 {
			ui.Text(c, "还没有已信任的主机。").FontSize(11).TextColor(t.TextMuted)
			return
		}
		ui.Column(c).FillWidth().Radius(8).Border(1, t.Border).Children(func() {
			for i, k := range hosts {
				name := k.Host
				if k.Port != 22 {
					name = fmt.Sprintf("%s:%d", k.Host, k.Port)
				}
				row := ui.Row(c).FillWidth().Height(34).Padding(0, 10).Gap(8).
					AlignItems(ui.Center)
				if i > 0 {
					row = row.BorderWidth(1, 0, 0, 0).BorderColor(t.Border)
				}
				row.Children(func() {
					ui.Icon(c, icoGlobe).FontSize(12).TextColor(t.TextMuted)
					mono(c, name).FontSize(11).TextColor(t.Text).Grow(1)
					if ui.Button(c, "忘记").Height(24).Padding(0, 8).Radius(5).
						FontSize(10.5).Clicked() {
						_ = a.hostKeys.Remove(k.Host, k.Port)
					}
				})
			}
		})
	})
}
