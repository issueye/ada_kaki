// The small shared controls: flat toolbar buttons, key and text
// chips, section headings, dialogs' blocks and secondary title bars.

package ui

import (
	"fmt"
	"strings"

	myui "github.com/egoist/mygo/ui"
)

// ghostButton is a flat toolbar button: no face at rest, a hover tint and
// a brighter glyph while the pointer is over it.
func ghostButton(c *myui.Context, svg *myui.SVG, name, tip string, fn func()) {
	t := c.Theme()
	b := myui.Button(c, "").Label(name).Padding(5).Radius(7).Shrink(0)
	b = b.Background(myui.Transparent)
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
		myui.Icon(c, svg).FontSize(14).TextColor(color)
	})
	if b.Clicked() {
		fn()
	}
}

// plainAction is a quiet action row: no face at rest, a hover tint, the
// label in the text color and the fast path's keys at its end. It opens
// a menu drawn in the window with the theme's look.
func plainAction(c *myui.Context, label, hint, tip string, svg *myui.SVG, build func(*myui.Menu)) {
	t := c.Theme()
	b := myui.Button(c, "").Label(label).Height(30).Padding(0, 8).Radius(6).Shrink(0)
	b = b.Background(myui.Transparent).BorderWidth(0)
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
		myui.Icon(c, svg).FontSize(13).TextColor(t.TextMuted)
		myui.Text(c, label).FontSize(12).FontWeight(500).TextColor(t.Text)
		myui.Spacer(c)
		if hint != "" {
			kbd(c, hint)
		}
	})
}

// kbdOn draws a key chip on a filled control, e.g. the fast path on the
// accent button.
func kbdOn(c *myui.Context, key string, bg, fg myui.Color) {
	myui.Box(c).Padding(2, 6).Radius(4).Background(bg).Children(func() {
		myui.Text(c, key).Font(monoFamily()).FontSize(9).TextColor(fg)
	})
}

// chip is a small tinted label: a key, a kind, a rule name.
func chip(c *myui.Context, text string, fg, bg myui.Color) myui.Element {
	return myui.Box(c).Padding(2, 6).Radius(4).Background(bg).Children(func() {
		myui.Text(c, text).FontSize(9.5).FontWeight(500).TextColor(fg)
	})
}

// kbd draws a keyboard key the way web UIs do: a small bordered chip.
func kbd(c *myui.Context, key string) {
	t := c.Theme()
	myui.Box(c).Padding(2, 6).Radius(4).Border(1, t.Border).Background(t.Background).Children(func() {
		myui.Text(c, key).Font(monoFamily()).FontSize(10).TextColor(t.TextMuted)
	})
}

// mono writes code-like text in the platform's monospaced face.
func mono(c *myui.Context, s string) myui.Element {
	return myui.Text(c, s).Font(monoFamily())
}

// dot is a filled state dot.
func dot(c *myui.Context, color myui.Color, size float32) {
	myui.Box(c).Width(size).Height(size).Radius(size / 2).Background(color).Shrink(0)
}

// sectionLabel is a small heading that groups rows of a pane or dialog.
func sectionLabel(c *myui.Context, text string) {
	myui.Text(c, text).FontSize(10.5).FontWeight(500).LetterSpacing(0.6).
		TextColor(c.Theme().TextMuted)
}

// sectionHeader is a sidebar section: its name, the count, a chevron.
func sectionHeader(c *myui.Context, title string, count int) {
	t := c.Theme()
	myui.Row(c).FillWidth().Height(26).Padding(0, 12, 0, 14).Gap(6).
		AlignItems(myui.Center).Children(func() {
		myui.Text(c, title).FontSize(10.5).FontWeight(500).TextColor(t.TextMuted)
		myui.Spacer(c)
		if count > 0 {
			myui.Text(c, fmt.Sprint(count)).Font(monoFamily()).FontSize(9.5).
				TextColor(t.TextMuted.Alpha(0.75))
		}
		myui.Icon(c, icoChevronDown).FontSize(11).TextColor(t.TextMuted.Alpha(0.7))
	})
}

// dialogTitle is the heading of a dialog: a glyph and a title, the glyph
// in the accent color or in the danger color for what interrupts.
func dialogTitle(c *myui.Context, svg *myui.SVG, title string, danger bool) {
	t := c.Theme()
	color := t.Accent
	if danger {
		color = t.Danger
	}
	myui.Row(c).Gap(10).AlignItems(myui.Center).Children(func() {
		myui.Icon(c, svg).FontSize(18).TextColor(color)
		myui.Text(c, title).FontSize(16).Bold()
	})
}

// codeBlock is the quiet surface that shows a command, a fingerprint or a
// config snippet.
func codeBlock(c *myui.Context, fn func()) {
	t := c.Theme()
	myui.Box(c).FillWidth().Padding(10).Radius(6).Background(t.Surface).
		Border(1, t.Border).Children(fn)
}

// verticalRule is the quiet divider between groups of title-bar actions.
func verticalRule(c *myui.Context) {
	myui.Box(c).Width(1).Height(16).Margin(2, 0).Background(c.Theme().Border).Shrink(0)
}

// emptyNote is a section's placeholder line, as in a sidebar with no
// sessions of that kind yet.
func emptyNote(c *myui.Context, text string) {
	myui.Text(c, text).FontSize(11).TextColor(c.Theme().TextMuted).Padding(4, 12)
}

// firstFamily is the first entry of a comma-separated font list.
func firstFamily(list string) string {
	if i := strings.IndexByte(list, ','); i >= 0 {
		list = list[:i]
	}
	return strings.TrimSpace(list)
}

// titleStrip is a secondary window's own title bar (TitleBarHidden): a
// quiet band with the title at left and extra controls at right, the
// whole strip dragging the window and keeping clear of the system's
// window controls.
func titleStrip(c *myui.Context, title string, right func()) {
	bar := c.TitleBar()
	t := c.Theme()
	myui.Row(c).FillWidth().Height(chromeBarHeight(c)).Padding(0, bar.Right+12, 0, bar.Left+14).
		Background(t.Surface).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).
		DragWindow().AlignItems(myui.Center).Gap(9).Children(func() {
		if title != "" {
			myui.Text(c, title).FontSize(13).FontWeight(600)
		}
		myui.Spacer(c)
		if right != nil {
			right()
		}
	})
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
