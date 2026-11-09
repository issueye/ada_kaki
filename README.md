# AdaKaki 控制台

一个用 Go 编写的桌面多路终端控制台，基于 [mygo](../mygo) 框架构建：

- **本地 Shell**：cmd / Windows PowerShell / PowerShell 7 / Git Bash / 自定义命令
- **多窗口管理**：主窗口标签页 + 侧边栏会话树；任何会话可"弹出"为独立窗口
- **SSH**：密码 / 私钥（口令）认证，连接档案持久化（Windows DPAPI 加密凭据）
- **MCP 协议**：为 AI Agent 提供 Model Context Protocol 服务端（stdio 与 HTTP 双传输）
- **终端模拟**：Ghostty 的 libghostty-vt（mygo `plugins/terminal`），原生 UI 绘制，无 WebView、无 cgo、单二进制

## 构建与运行

需要 Go 1.27+（本仓库依赖本地路径的 mygo，见 `go.mod` 的 `replace`）。

```sh
go build -o adakaki.exe .
./adakaki.exe            # 打开控制台
```

首次运行会从 mygo 的 GitHub Releases 自动下载 `ghostty-vt.dll`（SHA-256 校验）到用户缓存 `~\AppData\Local\mygo\natives\`；之后离线可用。发布时把 dll 放到 exe 旁即可完全免下载。

## 使用

- 左上 **＋ 新建**：选择 shell（cmd/PowerShell/Git Bash/自定义命令）或 SSH 连接
- 标签页：点击切换；右键：分屏 / 弹出同屏窗口 / 重新连接 / 关闭
- 终端上右键：复制选区 / 无选区粘贴（Windows Terminal 惯例）；分屏与关闭在标签、侧栏和空分格的右键菜单里
- **快捷键**：`Ctrl+Shift+T` 新建默认 shell · `Ctrl+Tab`/`Ctrl+Shift+Tab` 切换会话 · `Ctrl+Shift+W` 关闭会话 · `Ctrl+Shift+P` 弹出同屏窗口 · `Ctrl+F` 终端搜索（回车下一个，Esc 关闭）
- **会话恢复**：正常重启后自动还原上次打开的会话（SSH 档案会话自动重拨）
- **⚙ 设置页**（左右布局）：外观（终端配色方案，Ghostty 300+ 主题搜索选择）、终端（字体家族/字号实时生效）、会话（默认 shell、启动恢复开关）、MCP、安全（护栏/SSH 校验/已信任主机）
- **🛡**：MCP 审计日志查看
- 会话退出后灰显，可移除；SSH 断开可右键"重新连接"

## MCP：给 AI Agent 用

### stdio（推荐，适用于 Claude Desktop / ZCode / Cursor 等）

```json
{
  "mcpServers": {
    "adakaki": {
      "command": "E:/codes/go_projects/ada_kaki/adakaki.exe",
      "args": ["mcp"]
    }
  }
}
```

stdio 模式是独立进程（无 GUI），直接驱动真实的 shell 与 SSH 会话。

### HTTP（GUI 运行时）

GUI 启动后自动监听 `http://127.0.0.1:8731/mcp`（⚙ 设置可改端口或关闭）。Bearer token 在 `%AppData%\adakaki\token`。仅监听回环地址。

```yaml
mcp:
  servers:
    adakaki:
      url: http://127.0.0.1:8731/mcp
      headers:
        Authorization: Bearer <token>
```

### 工具（11 个）

| 工具 | 用途 |
|---|---|
| `list_shells` | 探测可用 shell |
| `list_sessions` | 列出会话（id/kind/标题/状态/退出码） |
| `create_session` | 新建本地会话（shell/cwd/自定义命令） |
| `ssh_connect` | SSH 连接（档案或 host/user/密码/密钥） |
| `list_ssh_profiles` | 已存 SSH 档案 |
| `write_session` | 写入终端（自带 `\r` 回车） |
| `read_session` | 增量读输出（游标式，可阻塞等待） |
| `screen_session` | 当前屏幕文本（看全屏程序） |
| `resize_session` | 调整终端尺寸 |
| `close_session` | 关闭会话 |
| `run_command` | **高频**：发命令 → 收输出（静默窗口/超时控制） |

典型流程：`list_shells` → `create_session` → `run_command("git status")` → 读输出 → `close_session`。

### 安全护栏与审计

AI Agent 的每一次工具调用都经过两道闸：

- **审计日志** `%AppData%\adakaki\audit.log`（JSON Lines，轮转保留）：时间、客户端、工具、**执行的命令**、目标会话、结果与耗时；密码类参数脱敏。侧栏 🛡 打开实时查看窗口。
- **危险命令护栏**（⚙ 设置可关）：内置规则——`format`/`diskpart`/`dd of=/dev` 等**直接拦截**；`rm -rf`/`del /s`/`Remove-Item -Recurse`/`git push --force`/关机类等**弹窗请求人工放行**（无 GUI 的 stdio 模式默认拒绝，可用 config.json 的 `guard_headless: "allow"` 放开，`guard_deny`/`guard_confirm` 可加自定义正则）。被拦截的调用同样记录在审计中。

### Agent 操作蒙版（人机冲突防护）

AI Agent 经 MCP 工具操作某个会话期间，该会话的终端分格自动盖上**半透明蒙版**：

- **能看**：终端实时画面从蒙版下透出，Spinner + 当前工具名（如 `run_command`）一目了然；侧栏/标签状态点变 🤖
- **不能扰**：蒙版抢占键盘焦点并拦截点击，人工输入（含同屏弹出窗口）静默丢弃；关闭会话/分格被拦截并提示；退出确认框会警示正在操作的 Agent 数
- 蒙版随工具调用结束自动消失，Agent 自己的 MCP 写入不受任何影响（走绕过锁的 `Send` 通道）

## 安全提示

- MCP 工具具备**任意命令执行**能力——这正是它的用途。HTTP 端点仅回环 + token 保护；不要把端口暴露到网络。
- **SSH 主机密钥 TOFU**：首次连接弹窗确认指纹并存入 `%AppData%\adakaki\known_hosts`（OpenSSH 兼容格式）；已知主机**指纹变化一律拒绝连接**（防中间人），可在 ⚙ 设置的"已信任主机"中"忘记"后重试；校验模式可在设置中调整（弹窗确认 / 自动信任新主机 / 关闭）。无 GUI 的 `adakaki mcp` 模式默认自动信任新主机。
- SSH 密码/口令经 Windows DPAPI（当前用户级）加密后落盘；同用户同机的其他进程理论上可解密（DPAPI 的设计边界）。
- SSH 断开后可一键重连（会话右键"重新连接"，或 MCP 工具 `reconnect_session`）：原会话 ID、分格与标签位置全部保留。

## 文档

- [设计文档](docs/DESIGN.md)：mygo 框架分析、架构、模块、协议、风险
- [开发计划](docs/DEVPLAN.md)：里程碑、验收标准、任务清单

## 测试

```sh
go build ./... && go vet ./... && go test ./... -count=1
```

覆盖：ConPTY/Ghostty headless 会话、引擎状态机与事件、shell 探测、DPAPI 往返、mock SSH 服务器（认证/shell/exec/超时）、MCP 协议（握手/工具闭环/未初始化拒绝）、MCP HTTP（鉴权/调用）。
