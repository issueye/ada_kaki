package main

// The app's own text is Chinese; the framework's menus and dialogs come
// in English only, so this catalog teaches them the words the app uses.
// P23 replaces it with a language setting.

import (
	"embed"
	"io/fs"
	"log"

	"github.com/egoist/mygo/i18n"
)

//go:embed locales/*.json
var localesFS embed.FS

// registerLocales adds the app's translations to the framework's.
func registerLocales() {
	sub, err := fs.Sub(localesFS, "locales")
	if err != nil {
		log.Printf("locales: %v", err)
		return
	}
	c, err := i18n.NewCatalog(sub, "zh")
	if err != nil {
		log.Printf("locales: %v", err)
		return
	}
	i18n.Register(c)
}
