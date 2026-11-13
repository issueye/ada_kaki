package main

// The app's web-style chrome: the palette, the shared metrics and the
// control looks of the main window, its dialogs and the secondary
// windows. The palette is "kaki" (persimmon) on ink: one warm accent
// against neutral surfaces, so terminal output keeps its own colors.

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/egoist/mygo/ui"
)

const (
	// chromeBarMin is the least height of a self-drawn title bar; the
	// system's controls band may need more.
	chromeBarMin = 40
	// tabBarHeight and tabPillHeight are the tab strip and its pills.
	tabBarHeight  = 40
	tabPillHeight = 30
	// statusBarHeight is the strip along the bottom.
	statusBarHeight = 28
	// sidebarWidth is the session column; settingsNavWidth the
	// settings window's category column.
	sidebarWidth     float32 = 260
	sidebarRailWidth float32 = 56
	settingsNavWidth float32 = 208
	// sidebarMinWidth and sidebarMaxWidth bound the drag of the session
	// column, so its rows never squeeze into a distorted shape.
	sidebarMinWidth float32 = 200
	sidebarMaxWidth float32 = 560
)

// colorMask is the agent mask's translucent veil over a terminal.
var colorMask = ui.RGBA(8, 10, 18, 0.72)

// webTheme lays the app's palette and web-like metrics over the window's
// theme: persimmon accent on ink (dark) or warm paper (light), rounder
// controls and scrollbars. The appearance itself is the framework's
// (mygo.Theme.SetSource), so the window controls and menus follow too.
func webTheme(c *ui.Context) {
	t := *c.Theme()
	if t.Dark {
		t.Background = ui.RGBA(14, 15, 18, 1)
		t.Surface = ui.RGBA(21, 23, 27, 1)
		t.SurfaceHover = ui.RGBA(34, 38, 45, 1)
		t.SurfacePressed = ui.RGBA(42, 46, 54, 1)
		t.Border = ui.RGBA(37, 41, 50, 1)
		t.Text = ui.RGBA(237, 234, 229, 1)
		t.TextMuted = ui.RGBA(145, 151, 161, 1)
		t.Accent = ui.RGBA(228, 99, 60, 1)
		t.AccentHover = ui.RGBA(240, 116, 76, 1)
		t.AccentPressed = ui.RGBA(201, 80, 44, 1)
		t.AccentText = ui.RGBA(26, 15, 11, 1)
		t.Danger = ui.RGBA(242, 99, 92, 1)
		t.Warning = ui.RGBA(224, 164, 74, 1)
		t.Success = ui.RGBA(79, 191, 106, 1)
		t.Selection = ui.RGBA(228, 99, 60, 0.28)
		t.Focus = t.Accent
		t.Scrollbar = ui.RGBA(46, 50, 58, 1)
		t.Inverse = ui.RGBA(237, 234, 229, 1)
		t.InverseText = ui.RGBA(18, 16, 14, 1)
	} else {
		t.Background = ui.RGBA(251, 250, 247, 1)
		t.Surface = ui.RGBA(242, 240, 234, 1)
		t.SurfaceHover = ui.RGBA(233, 229, 220, 1)
		t.SurfacePressed = ui.RGBA(225, 221, 212, 1)
		t.Border = ui.RGBA(225, 221, 212, 1)
		t.Text = ui.RGBA(30, 28, 24, 1)
		t.TextMuted = ui.RGBA(110, 106, 98, 1)
		t.Accent = ui.RGBA(194, 78, 43, 1)
		t.AccentHover = ui.RGBA(169, 69, 38, 1)
		t.AccentPressed = ui.RGBA(143, 58, 32, 1)
		t.AccentText = ui.RGBA(251, 247, 242, 1)
		t.Danger = ui.RGBA(192, 57, 43, 1)
		t.Warning = ui.RGBA(154, 106, 18, 1)
		t.Success = ui.RGBA(31, 122, 61, 1)
		t.Selection = ui.RGBA(194, 78, 43, 0.22)
		t.Focus = t.Accent
		t.Scrollbar = ui.RGBA(216, 211, 201, 1)
		t.Inverse = ui.RGBA(30, 28, 24, 1)
		t.InverseText = ui.RGBA(251, 250, 247, 1)
	}
	if t.Radius < 8 {
		t.Radius = 8
	}
	if t.ScrollbarWidth < 8 {
		t.ScrollbarWidth = 8
	}
	c.SetTheme(&t)
}

// monoFamily is the family of code-like text: the platform's own
// monospaced face, since the UI cannot assume a bundled font.
func monoFamily() string {
	switch runtime.GOOS {
	case "darwin":
		return "Menlo"
	case "linux":
		return "DejaVu Sans Mono"
	}
	return "Consolas"
}

// clampSidebarWidth keeps the session column inside its usable range.
func clampSidebarWidth(w float32) float32 {
	return min(max(w, sidebarMinWidth), sidebarMaxWidth)
}

// chromeBarHeight is the height of a self-drawn title bar: exactly the
// band the system's window controls take, so the bar and the buttons line
// up. 40 when the window shows no controls of its own.
func chromeBarHeight(c *ui.Context) float32 {
	if h := c.TitleBar().Height; h > 0 {
		return h
	}
	return chromeBarMin
}

// ghostButton is a flat toolbar button: no face at rest, a hover tint and
// a brighter glyph while the pointer is over it.
func ghostButton(c *ui.Context, svg *ui.SVG, name, tip string, fn func()) {
	t := c.Theme()
	b := ui.Button(c, "").Label(name).Padding(5).Radius(7).Shrink(0)
	b = b.Background(ui.Transparent)
	b = b.BorderWidth(0)
	hovered := b.Hovered()
	color := t.TextMuted
	if hovered {
		b = b.Background(t.SurfaceHover)
		color = t.Text
	}
	if tip != "" {
		b = b.Tooltip(tip)
	}
	b.Children(func() {
		ui.Icon(c, svg).FontSize(14).TextColor(color)
	})
	if b.Clicked() {
		fn()
	}
}

// plainAction is a quiet action row: no face at rest, a hover tint, the
// label in the text color and the fast path's keys at its end. It opens
// a menu drawn in the window with the theme's look.
func plainAction(c *ui.Context, label, hint, tip string, svg *ui.SVG, build func(*ui.Menu)) {
	t := c.Theme()
	b := ui.Button(c, "").Label(label).Height(30).Padding(0, 8).Radius(6).Shrink(0)
	b = b.Background(ui.Transparent).BorderWidth(0)
	if b.Hovered() {
		b = b.Background(t.SurfaceHover)
	}
	if tip != "" {
		b = b.Tooltip(tip)
	}
	if build != nil {
		b = b.PopoverMenuButton(build)
	}
	b.Children(func() {
		ui.Icon(c, svg).FontSize(13).TextColor(t.TextMuted)
		ui.Text(c, label).FontSize(12).FontWeight(500).TextColor(t.Text)
		ui.Spacer(c)
		if hint != "" {
			kbd(c, hint)
		}
	})
}

// kbdOn draws a key chip on a filled control, e.g. the fast path on the
// accent button.
func kbdOn(c *ui.Context, key string, bg, fg ui.Color) {
	ui.Box(c).Padding(2, 6).Radius(4).Background(bg).Children(func() {
		ui.Text(c, key).Font(monoFamily()).FontSize(9).TextColor(fg)
	})
}

// chip is a small tinted label: a key, a kind, a rule name.
func chip(c *ui.Context, text string, fg, bg ui.Color) ui.Element {
	return ui.Box(c).Padding(2, 6).Radius(4).Background(bg).Children(func() {
		ui.Text(c, text).FontSize(9.5).FontWeight(500).TextColor(fg)
	})
}

// kbd draws a keyboard key the way web UIs do: a small bordered chip.
func kbd(c *ui.Context, key string) {
	t := c.Theme()
	ui.Box(c).Padding(2, 6).Radius(4).Border(1, t.Border).Background(t.Background).Children(func() {
		ui.Text(c, key).Font(monoFamily()).FontSize(10).TextColor(t.TextMuted)
	})
}

// mono writes code-like text in the platform's monospaced face.
func mono(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).Font(monoFamily())
}

// dot is a filled state dot.
func dot(c *ui.Context, color ui.Color, size float32) {
	ui.Box(c).Width(size).Height(size).Radius(size / 2).Background(color).Shrink(0)
}

// sectionLabel is a small heading that groups rows of a pane or dialog.
func sectionLabel(c *ui.Context, text string) {
	ui.Text(c, text).FontSize(10.5).FontWeight(500).LetterSpacing(0.6).
		TextColor(c.Theme().TextMuted)
}

// sectionHeader is a sidebar section: its name, the count, a chevron.
func sectionHeader(c *ui.Context, title string, count int) {
	t := c.Theme()
	ui.Row(c).FillWidth().Height(26).Padding(0, 12, 0, 14).Gap(6).
		AlignItems(ui.Center).Children(func() {
		ui.Text(c, title).FontSize(10.5).FontWeight(500).TextColor(t.TextMuted)
		ui.Spacer(c)
		if count > 0 {
			ui.Text(c, fmt.Sprint(count)).Font(monoFamily()).FontSize(9.5).
				TextColor(t.TextMuted.Alpha(0.75))
		}
		ui.Icon(c, icoChevronDown).FontSize(11).TextColor(t.TextMuted.Alpha(0.7))
	})
}

// dialogTitle is the heading of a dialog: a glyph and a title, the glyph
// in the accent color or in the danger color for what interrupts.
func dialogTitle(c *ui.Context, svg *ui.SVG, title string, danger bool) {
	t := c.Theme()
	color := t.Accent
	if danger {
		color = t.Danger
	}
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, svg).FontSize(18).TextColor(color)
		ui.Text(c, title).FontSize(16).Bold()
	})
}

// codeBlock is the quiet surface that shows a command, a fingerprint or a
// config snippet.
func codeBlock(c *ui.Context, fn func()) {
	t := c.Theme()
	ui.Box(c).FillWidth().Padding(10).Radius(6).Background(t.Surface).
		Border(1, t.Border).Children(fn)
}

// verticalRule is the quiet divider between groups of title-bar actions.
func verticalRule(c *ui.Context) {
	ui.Box(c).Width(1).Height(16).Margin(2, 0).Background(c.Theme().Border).Shrink(0)
}

// emptyNote is a section's placeholder line, as in a sidebar with no
// sessions of that kind yet.
func emptyNote(c *ui.Context, text string) {
	ui.Text(c, text).FontSize(11).TextColor(c.Theme().TextMuted).Padding(4, 12)
}

// firstFamily is the first entry of a comma-separated font list.
func firstFamily(list string) string {
	if i := strings.IndexByte(list, ','); i >= 0 {
		list = list[:i]
	}
	return strings.TrimSpace(list)
}
