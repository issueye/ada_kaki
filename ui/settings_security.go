// The security category: the command guard, host key policy and the
// trusted hosts list.

package ui

import (
	"fmt"

	myui "github.com/egoist/mygo/ui"
)

func (a *app) securitySettings(c *myui.Context) {
	t := c.Theme()
	settingsSection(c, "命令护栏", func() {
		if myui.Checkbox(c, &a.store.Settings.GuardDisabled,
			"关闭危险命令护栏（rm -rf 等不再确认/拦截）").Changed() {
			a.guard.Enabled = !a.store.Settings.GuardDisabled
			a.saveSettingsNow()
		}
		if !a.store.Settings.GuardDisabled {
			builtin, custom := a.guard.Count()
			myui.Row(c).FillWidth().Gap(8).Children(func() {
				chip(c, fmt.Sprintf("内置 %d 条", builtin), t.TextMuted, t.Surface)
				chip(c, fmt.Sprintf("自定义 %d 条", custom), t.TextMuted, t.Surface)
			})
			hint(c, "拦截与放行均记录于审计。自定义规则写在 config.json 的 guard_deny / guard_confirm。")
		}
	})

	settingsSection(c, "SSH 主机密钥", func() {
		fieldRow(c, "未知主机", func() {
			myui.PopoverMenuButton(c, strictLabel(a.store.Settings.StrictHostKey), func(m *myui.Menu) {
				for _, v := range []struct{ val, label string }{
					{"prompt", "首次连接弹窗确认"},
					{"accept_new", "自动信任新主机"},
					{"off", "关闭校验（不安全）"},
				} {
					if m.Item(v.label).Chosen() {
						a.store.Settings.StrictHostKey = v.val
						a.saveSettingsNow()
					}
				}
			}).Grow(1)
		})
		hint(c, "已知主机的指纹一旦变化，连接一律拒绝（防中间人）。")

		hosts := a.hostKeys.List()
		if len(hosts) == 0 {
			myui.Text(c, "还没有已信任的主机。").FontSize(11).TextColor(t.TextMuted)
			return
		}
		myui.Column(c).FillWidth().Radius(8).Border(1, t.Border).Children(func() {
			for i, k := range hosts {
				name := k.Host
				if k.Port != 22 {
					name = fmt.Sprintf("%s:%d", k.Host, k.Port)
				}
				row := myui.Row(c).FillWidth().Height(34).Padding(0, 10).Gap(8).
					AlignItems(myui.Center)
				if i > 0 {
					row = row.BorderWidth(1, 0, 0, 0).BorderColor(t.Border)
				}
				row.Children(func() {
					myui.Icon(c, icoGlobe).FontSize(12).TextColor(t.TextMuted)
					mono(c, name).FontSize(11).TextColor(t.Text).Grow(1)
					if myui.Button(c, "忘记").Height(24).Padding(0, 8).Radius(5).
						FontSize(10.5).Clicked() {
						_ = a.hostKeys.Remove(k.Host, k.Port)
					}
				})
			}
		})
	})
}
