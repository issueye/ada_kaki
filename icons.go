package main

// The UI's icons: Lucide-style single-color SVGs (24x24, stroke 2, round
// caps), rendered by ui.Icon in the text color of wherever they sit, at
// any DPI. Lucide is ISC licensed; these paths follow its drawing.

import (
	"github.com/egoist/mygo/ui"
)

func iconSVG(inner string) *ui.SVG {
	return ui.MustParseSVG([]byte(
		`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` +
			inner + `</svg>`))
}

// Icons of the console, one variable per glyph.
var (
	icoPlus = iconSVG(`<path d="M5 12h14"/><path d="M12 5v14"/>`)

	icoSettings = iconSVG(`<line x1="21" x2="14" y1="4" y2="4"/><line x1="10" x2="3" y1="4" y2="4"/>` +
		`<line x1="21" x2="12" y1="12" y2="12"/><line x1="8" x2="3" y1="12" y2="12"/>` +
		`<line x1="21" x2="16" y1="20" y2="20"/><line x1="12" x2="3" y1="20" y2="20"/>` +
		`<line x1="14" x2="14" y1="2" y2="6"/><line x1="8" x2="8" y1="10" y2="14"/>` +
		`<line x1="16" x2="16" y1="18" y2="22"/>`)

	icoHistory = iconSVG(`<path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8"/>` +
		`<path d="M3 3v5h5"/><path d="M12 7v5l4 2"/>`)

	icoBot = iconSVG(`<path d="M12 8V4H8"/><rect width="16" height="12" x="4" y="8" rx="2"/>` +
		`<path d="M2 14h2"/><path d="M20 14h2"/><path d="M15 13v2"/><path d="M9 13v2"/>`)

	icoX = iconSVG(`<path d="M18 6 6 18"/><path d="m6 6 12 12"/>`)

	icoChevronUp = iconSVG(`<path d="m18 15-6-6-6 6"/>`)

	icoChevronDown = iconSVG(`<path d="m6 9 6 6 6-6"/>`)

	icoCheck = iconSVG(`<path d="M20 6 9 17l-5-5"/>`)

	icoCircleX = iconSVG(`<circle cx="12" cy="12" r="10"/><path d="m15 9-6 6"/><path d="m9 9 6 6"/>`)

	icoAlert = iconSVG(`<path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z"/>` +
		`<path d="M12 9v4"/><path d="M12 17h.01"/>`)

	icoTerminal = iconSVG(`<path d="m4 17 6-6-6-6"/><path d="M12 19h8"/>`)

	icoGlobe = iconSVG(`<circle cx="12" cy="12" r="10"/>` +
		`<path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20"/><path d="M2 12h20"/>`)

	icoSearch = iconSVG(`<circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/>`)

	icoAppWindow = iconSVG(`<path d="M2 6a2 2 0 0 1 2-2h16a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2Z"/>` +
		`<path d="M10 4v4"/><path d="M2 8h20"/><path d="M6 4v4"/>`)

	// icoDot is the state dot: filled, in the text color of its context.
	icoDot = ui.MustParseSVG([]byte(
		`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="currentColor">` +
			`<circle cx="12" cy="12" r="6"/></svg>`))
)
