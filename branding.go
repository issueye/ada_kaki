package main

// The app's identity images: the persimmon terminal mark for windows and
// the notification area, embedded so a single executable carries them.
// assets/icon.svg is the source drawing.

import (
	"embed"
	"log"

	"github.com/egoist/mygo"
)

//go:embed assets/icon.png assets/tray.png assets/tray-template.png
var brandAssets embed.FS

var (
	appIconPNG      = mustBrandAsset("assets/icon.png")
	trayIconPNG     = mustBrandAsset("assets/tray.png")
	trayTemplatePNG = mustBrandAsset("assets/tray-template.png")
)

func mustBrandAsset(name string) []byte {
	data, err := brandAssets.ReadFile(name)
	if err != nil {
		// The files are embedded at build time, so this is a broken build.
		panic("branding: " + name + ": " + err.Error())
	}
	return data
}

// iconWindow gives a window the app's mark in the title bar, taskbar and
// Alt-Tab switcher. macOS windows show no icon of their own.
func iconWindow(w *mygo.Window) {
	if w == nil {
		return
	}
	if err := w.SetIcon(appIconPNG); err != nil {
		log.Printf("window icon: %v", err)
	}
}
