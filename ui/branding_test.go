package ui

import (
	"bytes"
	"image/png"
	"testing"
)

// TestBrandAssets keeps the embedded images decodable and square: a
// broken or replaced file would otherwise only show up in a running app.
func TestBrandAssets(t *testing.T) {
	for _, asset := range []struct {
		name string
		data []byte
	}{
		{"icon", appIconPNG},
		{"tray", trayIconPNG},
		{"tray-template", trayTemplatePNG},
	} {
		cfg, err := png.DecodeConfig(bytes.NewReader(asset.data))
		if err != nil {
			t.Errorf("%s: %v", asset.name, err)
			continue
		}
		if cfg.Width != cfg.Height || cfg.Width == 0 {
			t.Errorf("%s: %dx%d, want a square", asset.name, cfg.Width, cfg.Height)
		}
	}
}
