package main

// The settings window: a nav of categories on the left, the category's
// content on the right. Every change applies live — the engine takes it
// at once — and saves to the config as it is made.

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

// settingsView lays the window out: nav | content.
func (a *app) settingsView(c *ui.Context) {
	ui.Split(c, &a.settingsNavW, func() { a.settingsNav(c) }, func() { a.settingsContent(c) }).Fill()
}

// settingsNav is the category list on the left.
func (a *app) settingsNav(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		ui.Row(c).Padding(12, 10).Children(func() {
			ui.Text(c, "设置").Bold().FontSize(15)
		})
		ui.Sidebar(c, &a.settingsCat, func() {
			for _, cat := range settingsCats {
				ui.SidebarItem(c.Key(cat), cat, nil, cat)
			}
		}).Fill()
	})
}

var settingsCats = []string{"外观", "终端", "会话", "MCP", "安全"}

// settingsContent builds the category the nav chose.
func (a *app) settingsContent(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		ui.Row(c).Padding(14, 12).Children(func() {
			ui.Text(c, a.settingsCat).Bold().FontSize(16)
		})
		ui.Divider(c)
		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).FillWidth().Padding(16, 12).Gap(12).Children(func() {
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

// fieldRow is a label with a control beside it, the settings rows' shape.
func fieldRow(c *ui.Context, label string, control func()) {
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).Width(150)
		control()
	})
}

// ---- 外观 ----

func (a *app) appearanceSettings(c *ui.Context) {
	t := c.Theme()
	sectionLabel(c, "终端配色方案")
	current := a.store.Settings.TerminalTheme
	if current == "" {
		current = "跟随窗口（浅色/深色自动切换）"
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "当前："+current).Grow(1)
		if ui.Button(c, "跟随窗口").Clicked() {
			a.store.Settings.TerminalTheme = ""
			a.applyTheme()
			a.saveSettingsNow()
		}
	})
	ui.Text(c, "选择后立即应用到全部终端，新会话同样使用。来自 Ghostty 的配色目录。").TextColor(t.TextMuted).FontSize(12)

	ui.Text(c, "搜索主题").FontSize(12)
	ui.TextInput(c.Key("theme-filter"), &a.themeFilter).Placeholder("输入名称过滤，如 catppuccin / dracula").AutoFocus()
	names := a.filteredThemes()
	ui.Text(c, fmt.Sprintf("%d 个主题", len(names))).TextColor(t.TextMuted).FontSize(12)
	ui.Scroll(c).Height(320).Children(func() {
		ui.Column(c).Gap(2).Children(func() {
			shown := 0
			for _, name := range names {
				if shown >= 60 {
					ui.Text(c, "…继续输入名称以缩小范围").TextColor(t.TextMuted).FontSize(12)
					break
				}
				shown++
				n := name // capture
				selected := a.store.Settings.TerminalTheme == name
				row := ui.Row(c.Key("theme-"+name)).Gap(8).Padding(6, 10).Radius(6).AlignItems(ui.Center)
				if selected {
					row = row.Background(t.Surface)
				}
				row.Children(func() {
					if selected {
						ui.Text(c, "●").TextColor(t.Accent).FontSize(9)
					} else {
						ui.Text(c, " ").FontSize(9)
					}
					ui.Text(c, n).Grow(1)
				})
				if row.Clicked() {
					a.store.Settings.TerminalTheme = n
					a.applyTheme()
					a.saveSettingsNow()
				}
			}
		})
	})
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
	sectionLabel(c, "字体")
	fieldRow(c, "字体家族", func() {
		in := ui.TextInput(c.Key("set-font-family"), &a.store.Settings.FontFamily).
			Placeholder("系统等宽字体").Grow(1)
		if in.Changed() || in.Submitted() {
			a.applyFont()
			a.saveSettingsNow()
		}
	})
	ui.Text(c, "逗号分隔的候选列表，如 Cascadia Code, Consolas, JetBrains Mono；留空用系统等宽字体。").TextColor(t.TextMuted).FontSize(12)
	fieldRow(c, "字号（DIP）", func() {
		in := ui.TextInput(c.Key("set-font-size"), &a.sizeText).Placeholder("默认 13").Width(80)
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
	ui.Text(c, "改动立即应用到全部运行中的终端。").TextColor(t.TextMuted).FontSize(12)
}

// ---- 会话 ----

func (a *app) sessionSettings(c *ui.Context) {
	sectionLabel(c, "新会话")
	fieldRow(c, "默认 Shell", func() {
		ui.MenuButton(c, a.defaultShellLabel(), func(m *ui.Menu) {
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
	ui.Text(c, "快捷键 Ctrl+Shift+T、分屏的新分格、启动恢复都用它。").FontSize(12).TextColor(c.Theme().TextMuted)

	sectionLabel(c, "启动")
	if ui.Checkbox(c, &a.store.Settings.RestoreDisabled, "启动时不恢复上次的会话").Changed() {
		a.saveSettingsNow()
	}
}

// ---- MCP ----

func (a *app) mcpSettings(c *ui.Context) {
	t := c.Theme()
	sectionLabel(c, "HTTP 端点")
	if ui.Checkbox(c, &a.store.Settings.MCPHTTP, "启动 MCP HTTP 端点（127.0.0.1，供 AI Agent 直连）").Changed() {
		a.saveSettingsNow()
	}
	if a.store.Settings.MCPHTTP {
		fieldRow(c, "端口", func() {
			in := ui.TextInput(c.Key("set-mcp-port"), &a.setMCPPort).Placeholder("8731").Width(120)
			if in.Changed() || in.Submitted() {
				var p int
				fmt.Sscanf(a.setMCPPort, "%d", &p)
				if p > 0 && p < 65536 {
					a.store.Settings.MCPHTTPPort = p
					a.saveSettingsNow()
				}
			}
		})
		ui.Text(c, "Token 在配置目录的 token 文件中；端口的修改重启后生效。").TextColor(t.TextMuted).FontSize(12)
	}
	sectionLabel(c, "stdio")
	ui.Text(c, "命令行 adakaki mcp 始终可用：AI 客户端以子进程方式驱动，无 GUI 时也能创建会话。").TextColor(t.TextMuted).FontSize(12)
}

// ---- 安全 ----

func (a *app) securitySettings(c *ui.Context) {
	t := c.Theme()
	sectionLabel(c, "命令护栏")
	if ui.Checkbox(c, &a.store.Settings.GuardDisabled, "关闭危险命令护栏（rm -rf 等不再确认/拦截）").Changed() {
		a.guard.Enabled = !a.store.Settings.GuardDisabled
		a.saveSettingsNow()
	}
	if !a.store.Settings.GuardDisabled {
		builtin, custom := a.guard.Count()
		ui.Text(c, fmt.Sprintf("内置规则 %d 条，自定义 %d 条；自定义规则在 config.json 的 guard_deny / guard_confirm。拦截与放行均记录于审计。", builtin, custom)).TextColor(t.TextMuted).FontSize(12)
	}

	sectionLabel(c, "SSH 主机密钥")
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "未知主机").Width(150)
		ui.MenuButton(c, strictLabel(a.store.Settings.StrictHostKey), func(m *ui.Menu) {
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
	ui.Text(c, "已知主机的指纹一旦变化，连接一律拒绝（防中间人）。").TextColor(t.TextMuted).FontSize(12)

	if hosts := a.hostKeys.List(); len(hosts) > 0 {
		ui.Text(c, "已信任主机").FontSize(12)
		for _, k := range hosts {
			name := k.Host
			if k.Port != 22 {
				name = fmt.Sprintf("%s:%d", k.Host, k.Port)
			}
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, name).Grow(1).FontSize(12)
				if ui.Button(c, "忘记").Clicked() {
					_ = a.hostKeys.Remove(k.Host, k.Port)
				}
			})
		}
	}
}
