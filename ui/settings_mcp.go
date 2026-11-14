// The MCP category: the HTTP endpoint, its port and the stdio note.

package ui

import (
	"fmt"

	"github.com/egoist/mygo"
	myui "github.com/egoist/mygo/ui"
)

func (a *app) mcpSettings(c *myui.Context) {
	t := c.Theme()
	port := a.store.Settings.MCPHTTPPort
	if port == 0 {
		port = 8731
	}
	settingsSection(c, "HTTP 端点", func() {
		if myui.Checkbox(c, &a.store.Settings.MCPHTTP, "启动 MCP HTTP 端点（仅回环地址）").
			Changed() {
			a.saveSettingsNow()
		}
		if a.store.Settings.MCPHTTP {
			fieldRow(c, "端口", func() {
				in := myui.TextInput(c.Key("set-mcp-port"), &a.setMCPPort).
					Placeholder("8731").Width(120)
				if in.Changed() || in.Submitted() {
					var p int
					fmt.Sscanf(a.setMCPPort, "%d", &p)
					if p > 0 && p < 65536 {
						a.store.Settings.MCPHTTPPort = p
						a.saveSettingsNow()
					}
				}
			})
			myui.Row(c).FillWidth().Padding(12).Radius(8).Background(t.Surface).
				Border(1, t.Border).AlignItems(myui.Center).Gap(8).Children(func() {
				dot(c, t.Success, 6)
				mono(c, fmt.Sprintf("http://127.0.0.1:%d/mcp", port)).
					FontSize(11).TextColor(t.Text).Grow(1)
				if myui.Button(c, "复制地址").Height(26).Padding(0, 9).Radius(6).
					FontSize(10.5).Clicked() {
					mygo.Clipboard.WriteText(fmt.Sprintf("http://127.0.0.1:%d/mcp", port))
					a.pendingToast = "MCP 地址已复制"
					a.refreshUI()
				}
			})
			hint(c, "Token 在配置目录的 token 文件中；端口修改重启后生效。仅监听 127.0.0.1。")
		}
	})

	settingsSection(c, "stdio", func() {
		myui.Text(c, "命令行 adakaki mcp 始终可用：AI 客户端以子进程方式驱动，无 GUI 时也能创建会话。").
			FontSize(11).TextColor(t.TextMuted).Wrap()
	})
}
