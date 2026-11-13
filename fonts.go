package main

// The app ships a terminal typeface: a CJK-capable programming font, so
// the grid looks the same on a machine with nothing installed. It is
// registered with the framework, which makes it available to Font and to
// the terminal's font setting by name.

import (
	"embed"
	"io/fs"
	"log"

	"github.com/egoist/mygo/ui"
)

// bundledFontFamily is the name the bundled typeface is registered under
// and offered as.
const bundledFontFamily = "Maple Mono NF CN"

//go:embed assets/fonts/*.ttf
var bundledFonts embed.FS

// registerBundledFonts adds the fonts shipped with the app.
func registerBundledFonts() {
	paths, err := fs.Glob(bundledFonts, "assets/fonts/*.ttf")
	if err != nil {
		log.Printf("fonts: %v", err)
		return
	}
	for _, p := range paths {
		data, err := bundledFonts.ReadFile(p)
		if err != nil {
			log.Printf("fonts: %s: %v", p, err)
			continue
		}
		if err := ui.RegisterFont(data, bundledFontFamily); err != nil {
			log.Printf("fonts: %s: %v", p, err)
		}
	}
}
