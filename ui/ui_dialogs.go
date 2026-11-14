// The main window's dialogs: quit, the agent's command guard, the
// first-meeting host key, SSH connect and a custom command.

package ui

import (
	"adakaki/internal/shells"
	"fmt"

	myui "github.com/egoist/mygo/ui"
)

// icoShield is the guard dialog's icon: a shield with a check.
var icoShield = iconSVG(`<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/><path d="m9 12 2 2 4-4"/>`)

// icoShieldX is a denied call in the audit list.
var icoShieldX = iconSVG(`<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/><path d="m9.5 9.5 5 5"/><path d="m14.5 9.5-5 5"/>`)

// mainDialogs is every dialog of the main window.
func (a *app) mainDialogs(c *myui.Context) {
	t := c.Theme()
	if a.showQuit {
		myui.Modal(c, &a.showQuit, func() {
			agents := a.agentBusy()
			myui.Column(c.Key("quit-dialog")).Width(380).Gap(12).Children(func() {
				dialogTitle(c, icoAlert, "退出 AdaKaki？", true)
				if agents > 0 {
					myui.Row(c).Gap(8).AlignItems(myui.Center).Children(func() {
						myui.Icon(c, icoBot).FontSize(14).TextColor(t.Danger)
						myui.Text(c, fmt.Sprintf("AI Agent 正在操作 %d 个会话，退出会中断它们", agents)).
							FontSize(12).TextColor(t.Danger)
					})
				}
				myui.Text(c, "还有会话在运行，退出会关闭它们。").FontSize(12.5).TextColor(t.TextMuted)
				myui.Row(c).Gap(8).Justify(myui.End).Children(func() {
					if myui.Button(c, "取消").Clicked() {
						a.showQuit = false
					}
					if myui.PrimaryButton(c, "退出").Clicked() {
						a.showQuit = false
						a.quit()
					}
				})
			})
		})
	}
	if a.showGuardAsk {
		m := myui.Modal(c, &a.showGuardAsk, func() {
			req, rule := a.guardReq, a.guardRule
			myui.Column(c.Key("guard-dialog")).Width(560).Gap(12).Children(func() {
				dialogTitle(c, icoShield, "AI Agent 请求执行受控命令", false)
				if req != nil {
					myui.Text(c, fmt.Sprintf("工具 %s · 客户端 %s · 会话 %s",
						req.Tool, orDash(req.Client), orDash(req.Session))).
						FontSize(11.5).TextColor(t.TextMuted)
				}
				codeBlock(c, func() {
					mono(c, orDash(req.Command)).FontSize(11.5).Wrap()
				})
				if rule != nil {
					myui.Text(c, "命中规则："+rule.Reason+"（"+rule.Source+"）").
						FontSize(11.5).TextColor(t.TextMuted)
				}
				myui.Text(c, "拒绝会返回错误给 Agent；允许只放行这一条命令。").
					FontSize(11.5).TextColor(t.TextMuted)
				myui.Row(c).Gap(8).Justify(myui.End).Children(func() {
					if myui.Button(c, "拒绝").Clicked() {
						a.answerGuard(false)
					}
					if myui.PrimaryButton(c, "允许本次").Clicked() {
						a.answerGuard(true)
					}
				})
			})
		})
		if m.Dismissed() {
			a.answerGuard(false) // Escape 或点击外部：当作拒绝
		}
	}
	if a.showHostKey {
		m := myui.Modal(c, &a.showHostKey, func() {
			myui.Column(c.Key("hostkey-dialog")).Width(460).Gap(12).Children(func() {
				dialogTitle(c, icoShield, "首次连接 "+a.hkHost, false)
				myui.Text(c, "无法确认主机身份，请核对指纹：").FontSize(12).TextColor(t.TextMuted)
				codeBlock(c, func() {
					myui.Column(c).Gap(5).Children(func() {
						mono(c, "SHA256:"+a.hkFP).FontSize(11).Wrap()
						myui.Text(c, "主机身份以此指纹为准。").FontSize(11).TextColor(t.TextMuted)
					})
				})
				myui.Row(c).Gap(6).AlignItems(myui.Center).Children(func() {
					myui.Icon(c, icoAlert).FontSize(13).TextColor(t.Danger)
					myui.Text(c, "以后该主机指纹若变化，连接将被拒绝。").
						FontSize(11.5).TextColor(t.Danger)
				})
				myui.Row(c).Gap(8).Justify(myui.End).Children(func() {
					if myui.Button(c, "拒绝").Clicked() {
						a.answerHostKey(false)
					}
					if myui.PrimaryButton(c, "信任并保存").Clicked() {
						a.answerHostKey(true)
					}
				})
			})
		})
		if m.Dismissed() {
			a.answerHostKey(false) // Escape 或点击外部：当作拒绝
		}
	}
	if a.showSSH {
		myui.Modal(c, &a.showSSH, func() {
			myui.Column(c.Key("ssh-dialog")).Width(460).Gap(12).Children(func() {
				dialogTitle(c, icoGlobe, "SSH 连接", false)
				if len(a.store.Profiles) > 0 {
					myui.PopoverMenuButton(c, "选择已存档案…", func(m *myui.Menu) {
						for _, p := range a.store.Profiles {
							if m.Item(p.Name).Chosen() {
								a.fillSSH(p)
							}
						}
					})
				}
				myui.Row(c).Gap(8).Children(func() {
					myui.TextInput(c.Key("ssh-host"), &a.sshHost).Placeholder("主机").Grow(1)
					myui.TextInput(c.Key("ssh-port"), &a.sshPort).Placeholder("端口").Width(90)
				})
				myui.TextInput(c.Key("ssh-user"), &a.sshUser).Placeholder("用户名")
				myui.PopoverMenuButton(c, "认证方式："+authLabel(a.sshAuth), func(m *myui.Menu) {
					for _, k := range []string{"password", "key", "agent"} {
						if m.Item(authLabel(k)).Chosen() {
							a.sshAuth = k
						}
					}
				})
				switch a.sshAuth {
				case "key":
					myui.TextInput(c.Key("ssh-keypath"), &a.sshKeyPath).Placeholder(`私钥路径，如 C:\Users\you\.ssh\id_ed25519`)
					myui.TextInput(c.Key("ssh-passphrase"), &a.sshPass).Placeholder("私钥口令（可空）").Password()
				case "agent":
					myui.Text(c, "使用系统 OpenSSH Agent（服务需在运行）。").
						FontSize(11.5).TextColor(t.TextMuted)
				default:
					myui.TextInput(c.Key("ssh-password"), &a.sshPass).Placeholder("密码").Password()
				}
				myui.Text(c, "首次连接会请求确认主机指纹。").
					FontSize(11.5).TextColor(t.TextMuted)
				myui.Checkbox(c, &a.sshSave, "保存为档案")
				if a.sshSave {
					myui.TextInput(c.Key("ssh-saveas"), &a.sshSaveAs).Placeholder("档案名称")
				}
				myui.Row(c).Gap(8).Justify(myui.End).Children(func() {
					if myui.Button(c, "取消").Clicked() {
						a.showSSH = false
					}
					if myui.PrimaryButton(c, "连接").Clicked() {
						a.connectSSH()
					}
				})
			})
		})
	}
	if a.showCustom {
		myui.Modal(c, &a.showCustom, func() {
			myui.Column(c.Key("custom-dialog")).Width(440).Gap(12).Children(func() {
				dialogTitle(c, icoTerminal, "自定义命令", false)
				myui.Text(c, "要在终端里运行的命令行，例如 ssh user@host").
					FontSize(12.5).TextColor(t.TextMuted)
				if myui.TextInput(c.Key("custom-cmd"), &a.customCmd).Placeholder("命令行").
					AutoFocus().Submitted() {
					a.startCustom()
				}
				myui.Row(c).Gap(8).Justify(myui.End).Children(func() {
					if myui.Button(c, "取消").Clicked() {
						a.showCustom = false
					}
					if myui.PrimaryButton(c, "打开").Clicked() {
						a.startCustom()
					}
				})
			})
		})
	}
}

func (a *app) startCustom() {
	cmd := a.customCmd
	a.showCustom = false
	a.customCmd = ""
	if cmd != "" {
		a.newSession(shells.Custom, "", cmd)
	}
}
