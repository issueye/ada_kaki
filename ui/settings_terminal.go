// The terminal category: the font family and size, with the live
// preview.

package ui

import (
	"fmt"
	"strings"

	myui "github.com/egoist/mygo/ui"
)

func (a *app) terminalSettings(c *myui.Context) {
	t := c.Theme()
	settingsSection(c, "字体", func() {
		fieldRow(c, "字体", func() {
			a.fontPicker(c)
		})
		fieldRow(c, "自定义", func() {
			in := myui.TextInput(c.Key("set-font-family"), &a.store.Settings.FontFamily).
				Placeholder("逗号分隔候选，如 Cascadia Code, Consolas").Grow(1)
			if in.Changed() || in.Submitted() {
				a.applyFont()
				a.saveSettingsNow()
			}
		})
		hint(c, "列表里是系统等宽字体与内置字体；「自定义」可写回退列表，留空用系统等宽。")
		fieldRow(c, "字号（DIP）", func() {
			in := myui.TextInput(c.Key("set-font-size"), &a.sizeText).
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
		myui.Row(c).FillWidth().Padding(14).Radius(8).Background(t.Surface).
			Border(1, t.Border).Children(func() {
			myui.Text(c, "PS> go test ./...").Font(family).FontSize(float32(size) + 1).
				TextColor(t.Text)
		})
		hint(c, fmt.Sprintf("预览：%s · %d DIP", family, size))
	})
}

// fontPicker is the font family list: a menu button whose label is the
// current family, the bundled fonts marked, the system's after them.
func (a *app) fontPicker(c *myui.Context) {
	t := c.Theme()
	pick := myui.Button(c, "").Label(a.fontLabel()).Height(30).Padding(0, 10).
		Radius(6).Grow(1).Background(t.Surface).Border(1, t.Border)
	if pick.Hovered() {
		pick = pick.Background(t.SurfaceHover)
	}
	pick.PopoverMenuButton(func(m *myui.Menu) {
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
		myui.Text(c, a.fontLabel()).FontSize(11.5).TextColor(t.Text).Grow(1).
			SingleLine().Ellipsis("…")
		myui.Icon(c, icoChevronDown).FontSize(11).TextColor(t.TextMuted)
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
func (a *app) monoFamilies() []myui.FontFamily {
	if a.fontFamilies == nil {
		for _, f := range myui.FontFamilies() {
			if f.Monospaced {
				a.fontFamilies = append(a.fontFamilies, f)
			}
		}
	}
	return a.fontFamilies
}
