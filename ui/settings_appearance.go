// The appearance category: the interface scheme and the Ghostty
// terminal palettes.

package ui

import (
	"fmt"
	"strings"

	myui "github.com/egoist/mygo/ui"
)

func (a *app) appearanceSettings(c *myui.Context) {
	t := c.Theme()
	current := a.store.Settings.TerminalTheme
	label := current
	if label == "" {
		label = "跟随窗口（浅色/深色自动切换）"
	}
	settingsSection(c, "界面主题", func() {
		seg := myui.Row(c).Width(312).Height(34).Padding(3).Gap(2).Radius(8).
			Background(t.Surface).Border(1, t.Border)
		seg.Children(func() {
			options := []struct{ id, label string }{
				{"", "跟随系统"}, {"light", "浅色"}, {"dark", "深色"},
			}
			for _, o := range options {
				active := a.store.Settings.Appearance == o.id
				b := myui.Button(c, o.label).Height(28).Grow(1).Radius(6).FontSize(11)
				if active {
					b = b.Background(t.SurfacePressed).FontWeight(500)
				} else {
					b = b.Background(myui.Transparent)
				}
				if b.Clicked() {
					a.setAppearance(o.id)
				}
			}
		})
		hint(c, "强制浅色或深色会同时切换终端配色；选了具体配色方案时以方案为准。")
	})

	settingsSection(c, "终端配色方案", func() {
		myui.Row(c).FillWidth().Padding(12).Gap(12).Radius(8).
			Background(t.Surface).Border(1, t.Border).
			AlignItems(myui.Center).Children(func() {
			a.themeSwatch(c, t)
			myui.Column(c).Gap(3).Grow(1).Children(func() {
				myui.Text(c, label).FontSize(12.5).FontWeight(500)
				myui.Text(c, fmt.Sprintf("Ghostty 主题目录 · %d 个方案", len(a.ghosttyThemes))).
					FontSize(10.5).TextColor(t.TextMuted)
			})
			if current != "" {
				if myui.Button(c, "跟随窗口").Height(28).Padding(0, 10).Radius(6).
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
		myui.Row(c).FillWidth().Height(32).Padding(0, 10).Gap(7).Radius(7).
			Background(t.Background).Border(1, t.Border).
			AlignItems(myui.Center).Children(func() {
			myui.Icon(c, icoSearch).FontSize(12).TextColor(t.TextMuted)
			myui.TextInput(c.Key("theme-filter"), &a.themeFilter).Grow(1).Height(26).
				FontSize(11.5).BorderWidth(0).Background(myui.Transparent).
				Placeholder("输入名称过滤，如 catppuccin / dracula").
				PlaceholderColor(t.TextMuted).AutoFocus()
			if a.themeFilter != "" {
				if myui.Button(c, "").Label("清除").Padding(2).Radius(4).
					Background(myui.Transparent).BorderWidth(0).Children(func() {
					myui.Icon(c, icoX).FontSize(11).TextColor(t.TextMuted)
				}).Clicked() {
					a.themeFilter = ""
				}
			}
		})
		names := a.filteredThemes()
		myui.Text(c, fmt.Sprintf("%d 个方案", len(names))).FontSize(10).TextColor(t.TextMuted)
		myui.Box(c).FillWidth().Height(300).Radius(8).Background(t.Background).
			Border(1, t.Border).Children(func() {
			myui.Scroll(c).Fill().Padding(4).Children(func() {
				myui.Column(c).FillWidth().Gap(1).Children(func() {
					shown := 0
					for _, name := range names {
						if shown >= 60 {
							myui.Text(c, "…继续输入名称以缩小范围").FontSize(10.5).
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
func (a *app) themeSwatch(c *myui.Context, t *myui.Theme) {
	colors := []myui.Color{t.Surface, t.Accent, t.Success, t.Warning}
	myui.Row(c).Width(44).Height(30).Gap(3).AlignItems(myui.Center).Shrink(0).Children(func() {
		for _, col := range colors {
			myui.Box(c).Width(9).Height(24).Radius(3).Background(col)
		}
	})
}

// themeRow is one scheme in the catalog.
func (a *app) themeRow(c *myui.Context, name string) {
	t := c.Theme()
	selected := a.store.Settings.TerminalTheme == name
	row := myui.Row(c).FillWidth().Height(30).Padding(0, 8).Gap(8).
		Radius(5).AlignItems(myui.Center)
	switch {
	case selected:
		row = row.Background(t.Accent.Alpha(0.12))
	case row.Hovered():
		row = row.Background(t.Surface)
	}
	row.Children(func() {
		if selected {
			myui.Icon(c, icoCheck).FontSize(11).TextColor(t.Accent)
		} else {
			myui.Box(c).Width(11)
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
