// Package ui implements AdaKaki's native console: the main window, its
// dialogs and secondary windows, the settings page, the tray, and the
// app state that drives them.
//
// main.go holds only the entry points; everything that draws or drives
// the console lives here. The files group by responsibility:
//
//   - app*.go: state and lifecycle, sessions, tabs and panes, dialogs'
//     host side, secondary windows, and the find state
//   - ui_main.go, ui_header.go, ui_sidebar.go, ui_tabs.go, ui_panes.go:
//     the main window's frame, chrome, sidebar, tab strip and split tree
//   - ui_pane_card.go, ui_dialogs.go, ui_menus.go, ui_find.go: the pane
//     cards, dialogs, context menus and search overlay
//   - ui_audit.go, ui_popout.go, tray.go, controlplane.go: the audit
//     window, synchronized popouts, the notification area and the MCP
//     HTTP control plane
//   - settings_*.go: one settings category per file, over the shared
//     settings_view.go frame
//   - ui_theme.go, ui_widgets.go: the persimmon palette, metrics and the
//     small controls shared by every window
//   - branding.go, fonts.go, icons.go, locales.go: the app's identity,
//     bundled font, icon set and framework text
//
// New UI code belongs in the file matching its responsibility, so no
// single file grows into a monolith again.
package ui
