// The app's web-style palette, metrics and the shared measures of
// the chrome.

package ui

import (
	"runtime"

	myui "github.com/egoist/mygo/ui"
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
	// paneGap is the room between panes, paneHeaderHeight the strip over
	// each pane's terminal, and paneRadius the panes' corners.
	paneGap          float32 = 8
	paneHeaderHeight float32 = 34
	paneRadius       float32 = 12
	// sidebarMinWidth and sidebarMaxWidth bound the drag of the session
	// column, so its rows never squeeze into a distorted shape.
	sidebarMinWidth float32 = 200
	sidebarMaxWidth float32 = 560
)

// colorMask is the agent mask's translucent veil over a terminal.
var colorMask = myui.RGBA(8, 10, 18, 0.72)

// webTheme lays the app's palette and web-like metrics over the window's
// theme: persimmon accent on ink (dark) or warm paper (light), rounder
// controls and scrollbars. The appearance itself is the framework's
// (mygo.Theme.SetSource), so the window controls and menus follow too.
func webTheme(c *myui.Context) {
	t := *c.Theme()
	if t.Dark {
		t.Background = myui.RGBA(14, 15, 18, 1)
		t.Surface = myui.RGBA(21, 23, 27, 1)
		t.SurfaceHover = myui.RGBA(34, 38, 45, 1)
		t.SurfacePressed = myui.RGBA(42, 46, 54, 1)
		t.Border = myui.RGBA(37, 41, 50, 1)
		t.Text = myui.RGBA(237, 234, 229, 1)
		t.TextMuted = myui.RGBA(145, 151, 161, 1)
		t.Accent = myui.RGBA(228, 99, 60, 1)
		t.AccentHover = myui.RGBA(240, 116, 76, 1)
		t.AccentPressed = myui.RGBA(201, 80, 44, 1)
		t.AccentText = myui.RGBA(26, 15, 11, 1)
		t.Danger = myui.RGBA(242, 99, 92, 1)
		t.Warning = myui.RGBA(224, 164, 74, 1)
		t.Success = myui.RGBA(79, 191, 106, 1)
		t.Selection = myui.RGBA(228, 99, 60, 0.28)
		t.Focus = t.Accent
		t.Scrollbar = myui.RGBA(46, 50, 58, 1)
		t.Inverse = myui.RGBA(237, 234, 229, 1)
		t.InverseText = myui.RGBA(18, 16, 14, 1)
	} else {
		t.Background = myui.RGBA(251, 250, 247, 1)
		t.Surface = myui.RGBA(242, 240, 234, 1)
		t.SurfaceHover = myui.RGBA(233, 229, 220, 1)
		t.SurfacePressed = myui.RGBA(225, 221, 212, 1)
		t.Border = myui.RGBA(225, 221, 212, 1)
		t.Text = myui.RGBA(30, 28, 24, 1)
		t.TextMuted = myui.RGBA(110, 106, 98, 1)
		t.Accent = myui.RGBA(194, 78, 43, 1)
		t.AccentHover = myui.RGBA(169, 69, 38, 1)
		t.AccentPressed = myui.RGBA(143, 58, 32, 1)
		t.AccentText = myui.RGBA(251, 247, 242, 1)
		t.Danger = myui.RGBA(192, 57, 43, 1)
		t.Warning = myui.RGBA(154, 106, 18, 1)
		t.Success = myui.RGBA(31, 122, 61, 1)
		t.Selection = myui.RGBA(194, 78, 43, 0.22)
		t.Focus = t.Accent
		t.Scrollbar = myui.RGBA(216, 211, 201, 1)
		t.Inverse = myui.RGBA(30, 28, 24, 1)
		t.InverseText = myui.RGBA(251, 250, 247, 1)
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
func chromeBarHeight(c *myui.Context) float32 {
	if h := c.TitleBar().Height; h > 0 {
		return h
	}
	return chromeBarMin
}
