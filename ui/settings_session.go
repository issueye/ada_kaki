// The session category: the default shell and startup restore.

package ui

import (
	myui "github.com/egoist/mygo/ui"
)

func (a *app) sessionSettings(c *myui.Context) {
	settingsSection(c, "新会话", func() {
		fieldRow(c, "默认 Shell", func() {
			myui.PopoverMenuButton(c, a.defaultShellLabel(), func(m *myui.Menu) {
				for _, sh := range a.shells {
					if sh.Found {
						if m.Item(sh.Name).Checked(a.store.Settings.DefaultShell == string(sh.ID)).Chosen() {
							a.store.Settings.DefaultShell = string(sh.ID)
							a.saveSettingsNow()
						}
					}
				}
			}).Grow(1)
		})
		hint(c, "快捷键 Ctrl+Shift+T、分屏的新分格、启动恢复都用它。")
	})

	settingsSection(c, "启动", func() {
		if myui.Checkbox(c, &a.store.Settings.RestoreDisabled, "启动时不恢复上次的会话").
			Changed() {
			a.saveSettingsNow()
		}
		hint(c, "关闭后，重启时按上次的标签与分格原样恢复；SSH 档案会话会自动重拨。")
	})
}
