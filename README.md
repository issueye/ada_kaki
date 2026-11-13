# AdaKaki 控制台

一个用 Go 编写的桌面多路终端控制台，基于 [mygo](../mygo) 框架构建：

- **本地 Shell**：cmd / Windows PowerShell / PowerShell 7 / Git Bash / 自定义命令
- **多窗口管理**：主窗口标签页 + 侧边栏会话树；任何会话可"弹出"为独立窗口
- **SSH**：密码 / 私钥（口令）认证，连接档案持久化（Windows DPAPI 加密凭据）
- **MCP 协议**：为 AI Agent 提供 Model Context Protocol 服务端（stdio 与 HTTP 双传输）
- **终端模拟**：Ghostty 的 libghostty-vt（mygo `plugins/terminal`），原生 UI 绘制，无 WebView、无 cgo、单二进制

## 字体

仓库自带 `assets/fonts/MapleMonoNormal-NF-CN-Regular.ttf`（Maple Mono NF CN：含中文的编程等宽字体），编译时嵌入二进制并在启动时注册，无需系统安装。设置 → 终端 → 字体 里可直接选「Maple Mono NF CN（内置）」；留空则用系统等宽字体。

## 构建与运行

需要 Go 1.27+，依赖 mygo v0.8.3（见 `go.mod`）。

```sh
# Windows：GUI 子系统，启动时不弹控制台窗口
.\build_windows.ps1

# 或直接：
go build -ldflags="-H windowsgui" -o adakaki.exe .
.\adakaki.exe            # 打开控制台
```

Linux/macOS 用普通 `go build -o adakaki .`；`-H windowsgui` 只适用于 Windows，用于避免多出一个黑色控制台窗口。调试时若想看日志可临时用普通 `go build`。

Windows 的 exe 图标由仓库里的 `rsrc_windows_amd64.syso` / `rsrc_windows_arm64.syso` 在构建时自动链接，无需额外工具；改图标后按 `assets/gen_icons.py` 顶部注释重新生成 ICO 与 syso。

首次运行会从 mygo 的 GitHub Releases 自动下载 `ghostty-vt.dll`（SHA-256 校验）到用户缓存 `~\AppData\Local\mygo\natives\`；之后离线可用。发布时把 dll 放到 exe 旁即可完全免下载。

## 使用

- 侧栏 **新建会话**（或 `Ctrl+Shift+T`）：菜单里选 shell（cmd/PowerShell/Git Bash/自定义命令）或 SSH 连接
- 侧栏 **筛选会话**：输入即过滤本地 / SSH 两个分组；分组标题显示数量
- 侧栏可 **拖拽分界线** 调整宽度（200–560，低于下限自动停住），点品牌行右侧图标或 `Ctrl+B` **收起/展开**；收起后是一条 **56px 图标栏**：品牌标记兼任展开按钮、柿色实心「新建」+ 透明「搜索」、每个会话一个 36×36 方块（类型图标 + 右下状态点，激活项带柿色边）、底部只留 MCP 状态点；所有按钮统一网格，提示走右侧 tooltip，右键仍是会话菜单
- 标签页：一个标签就是一个工作区，可容纳多个分格终端；标签名取聚焦终端的短名（如 cmd / PowerShell），多分格时显示 ×N；点标签切换，关闭钮关闭整个标签页里的全部会话；右键：新建标签页 / 关闭标签页 / 关闭其他标签页
- **分格卡片**（参考 gorex）：每个分格是一张圆角卡片，卡片头显示状态点、类型图标、标题与工作目录，右侧「向右分屏 / 向下分屏 / 放大 / 关闭」四个按钮始终显示，空间不足时省略标题与目录，不会挤掉按钮；卡片之间留 8px 缝隙，缝隙可拖动改变比例、双击恢复均分；点卡头聚焦，双击卡头放大该分格，再双击还原。会话右键（侧栏与折叠图标栏）另有 **强制关闭会话**，连同子进程立即结束，适合不响应 Ctrl+C 的 Java 服务
- 终端上右键：**复制 / 粘贴 / 全选 / 清空屏幕**，与应用内菜单同样式（窗口内绘制，非系统菜单）；分屏与关闭在标签、侧栏和空分格的右键菜单里
- **快捷键**：`Ctrl+Shift+T` 新建默认 shell · `Ctrl+Tab`/`Ctrl+Shift+Tab` 切换标签页 · `Ctrl+Shift+W` 关闭标签页 · `Ctrl+Shift+F12` 强制关闭当前分格（连同子进程） · `Ctrl+Shift+P` 弹出同屏窗口 · `Ctrl+B` 收起/展开侧栏 · `Ctrl+F` 终端搜索（浮层搜索条，回车下一个，Esc 关闭）
- **系统托盘**：通知区有一颗柿色终端图标；左键唤起并聚焦主窗口，右键菜单为「打开主窗口 / 新建会话 / 退出 AdaKaki」。菜单是应用内自绘的浮层（与主界面同款 Surface / 图标 / 悬停 / 字号），不是系统菜单；Linux 的 AppIndicator 只支持原生菜单，故那里仍用系统菜单兜底。退出会先走关闭确认（有会话运行时不会直接杀掉）。Windows 默认收进托盘溢出区，可从 `^` 中拖出固定
- **退出即清理**：Windows 下应用加入带 `KILL_ON_JOB_CLOSE` 的 Job Object；主窗口关闭、托盘退出、甚至被任务管理器强制结束后，本程序启动的所有 shell 与命令子进程都会随之结束，不会留下孤儿进程
- **应用图标**：任务栏、窗口与 exe 文件统一使用柿色终端标记；`assets/icon.svg` 是矢量源文件，`python assets/gen_icons.py`（需 Pillow）重新生成 PNG/ICO/托盘图，`rsrc_windows_*.syso` 把 ICO 嵌进 exe
- **会话恢复**：正常重启后自动还原上次打开的会话（SSH 档案会话自动重拨）
- **设置**（标题条滑杆图标）：左分类右内容，改动即存——外观（终端配色方案，Ghostty 600+ 主题搜索选择）、终端（字体选择器：内置 **Maple Mono NF CN** + 系统等宽字体，亦可自定义回退列表；字号实时生效并带预览）、会话（默认 shell、启动恢复开关）、MCP（端点/端口/地址复制）、安全（护栏/SSH 校验/已信任主机）
- **审计**（标题条盾牌图标）：MCP 调用列表 + 右侧详情，按结果（成功/失败/被拦截）筛选、按命令或客户端搜索，可复制单条 JSONL 或全部筛选结果
- 会话退出后灰显；右键"重新连接"可原地重开本地会话（同一 shell/命令/目录），SSH 断开同样可重连
- **界面**：柿色（persimmon）强调色，深/浅两套配色可跟随系统、在设置里强制切换，或点标题条右侧的太阳/月亮图标一键切换（终端一并跟随）；标题条与底部状态条贯通整个窗口，品牌/折叠、会话上下文、窗口操作都在同一条顶栏，MCP/会话/stdio 都在同一条底栏，侧栏或折叠图标栏只在两者之间伸缩；标题条高度与系统窗口按钮同高；下拉菜单为应用内绘制，不再弹系统菜单

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
- 会话退出后可一键重连（会话右键"重新连接"，或 MCP 工具 `reconnect_session`）：本地会话原地重开同一 shell/命令，SSH 会话重新拨号；原会话 ID、分格与标签位置全部保留。

## 文档

- [设计文档](docs/DESIGN.md)：mygo 框架分析、架构、模块、协议、风险
- [路线图](docs/ROADMAP.md)：v0.2 → v1.0 长期任务计划（P15–P25）
- [UI 评审](docs/UI-REVIEW.md)：界面评审结论与整改记录
- [开发计划](docs/DEVPLAN.md)：P0–P14 里程碑、验收标准、任务清单（已完成）

## 测试

```sh
go build ./... && go vet ./... && go test ./... -count=1
```

覆盖：ConPTY/Ghostty headless 会话、引擎状态机与事件、shell 探测、DPAPI 往返、mock SSH 服务器（认证/shell/exec/超时）、MCP 协议（握手/工具闭环/未初始化拒绝）、MCP HTTP（鉴权/调用）。
