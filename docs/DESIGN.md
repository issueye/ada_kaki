# AdaKaki 控制台 — 设计文档

> 版本：v2.0（2026-11-09）—— 架构改为 mygo `plugins/terminal`（Ghostty + 原生 UI）
> 状态：方案 A 已验证（headless 链路测试通过），待开发
> 关联文档：[开发计划](./DEVPLAN.md)

---

## 1. 项目概述

**AdaKaki** 是一个用 Go 开发的桌面多路终端控制台，运行于 Windows，基于本地框架 mygo 构建。

核心能力：

1. **本地 Shell 会话**：可创建 `cmd`、`PowerShell`（powershell/pwsh）、`Git Bash` 会话，亦可自定义命令。
2. **多窗口/多会话管理**：主窗口内以标签页组织多个会话；任何会话可"弹出"为独立窗口（Ghostty Mirror 同屏）；侧边栏集中管理全部会话。
3. **SSH 远程连接**：主机档案（host/port/user/密码/私钥）持久化；SSH 会话与本地会话共用同一终端 UI 与引擎。
4. **MCP 协议网关**：对 AI Agent 暴露 Model Context Protocol 服务端（stdio 与 Streamable HTTP 双传输），AI 可创建/驱动/读取终端会话。

非目标（v1 不做）：分屏布局、tmux 式会话迁移、串口/WSL、录屏回放、插件系统。

## 2. 背景：mygo 框架分析（选型依据）

### 2.1 框架结论

对 `E:\codes\go_projects\mygo` 的分析结论：mygo 是**纯 Go、无 cgo** 的桌面应用框架，本项目直接复用它。

| 维度 | 结论 |
|---|---|
| 渲染模式 | 每窗口可显示 **Web 页面**（系统 WebView）或**原生 UI**（`ui` 包，Go 自绘 GPU 渲染：Direct3D 11 on Windows）；同一应用可混用 |
| IPC | 类型化绑定 `mygo.Bind`；`*mygo.Channel[T]` 流式推送；`mygo.NewEvent[T]` 类型化事件（本项目原生 UI 不经 IPC，直接调 Go） |
| 生命周期 | `App.WhenReady` + `App.Run`；`NewWindow(WindowOptions{Content: ui.View(...)})` |
| 线程模型 | 后端方法/回调在主线程；公共方法须任意 goroutine 安全（`onMain`/`postMain`） |

### 2.2 终端插件（plugins/terminal）——方案 A 的基石

`examples/terminal`（控制台示例）背后是 `plugins/terminal`：用 **Ghostty 的 libghostty-vt** 做终端模拟，运行时经 purego 加载 `ghostty-vt.dll`（无 cgo），UI 用 `ui` 包绘制。

**关键 API（均已在本机验证）**：

| API | 用途 | 本项目用法 |
|---|---|---|
| `terminal.New(Options{Command, Dir, Env, Conn, OnTitle, OnExit, ...})` | 创建终端并启动程序；**程序在无视图时也能启动**（默认 80×24，0.5s 后） | 每个会话一个 Terminal；MCP 无头会话直接用 |
| `Options.Conn io.ReadWriteCloser` | 把终端接到**任意字节流**（文档明示 remote shell 场景）；带 `Resize(cols,rows)` 方法则跟随尺寸 | **SSH 会话**：自研 `x/crypto/ssh` 实现此接口接入 |
| `Options.ResizeByConn` | 尺寸由 Conn 持有方决定（多路复用器场景） | SSH 会话视情况使用 |
| `term.Send(data)` | 向程序写入（如同键入） | UI 键盘输入转发；MCP `write_session` |
| `term.Feed(data)` | 向模拟器注入输出字节 | 带外提示信息（如连接状态横幅） |
| `term.Text()` | 屏幕+回滚的**纯文本** | MCP `read_session`/`screen_session` |
| `term.History()` | 回滚字节 | MCP 读取扩展 |
| `term.Snapshot()` | 屏幕状态重编码为 VT 序列（重放式快照） | MCP `snapshot_session`（AI 想看样式时） |
| `term.Resize(cols, rows)` / `Size()` | 无视图终端调尺寸；有视图由视图决定 | 弹出/恢复时维持尺寸 |
| `term.Done()` / `ExitCode()` / `Title()` / `Dir()` | 退出通知/退出码/标题（OSC）/工作目录（OSC 7） | 会话状态机与 UI 标题 |
| `term.NewMirror()` + `Mirror.Frame()` | 把主终端屏幕重编码为 VT 帧，喂给**第二个终端**（`Conn` 模式），输入可回传主终端 `Send` | **多窗口同会话**：弹出窗口显示 Mirror 视图 |
| `Load()` / `LibraryPath()` | 加载 dll；未打包程序首次运行**自动下载**（GitHub Releases + SHA-256 校验，缓存 `<cache>/mygo/natives/<sha256>/`；`MYGO_GHOSTTY_VT` 环境变量或 exe 旁文件可覆盖） | 开发期自动下载；发布时 vendor 到 exe 旁 |

**已验证结论（2026-11-09，本机实测）**：`go test` 内 headless 创建 `cmd /c echo` 终端——首次运行自动下载 `ghostty-vt-windows-amd64.dll` 到 `C:\Users\issue\AppData\Local\mygo\natives\<sha256>\`，ConPTY 启动、Ghostty 模拟、`Text()` 读屏、`Done()`/`ExitCode()` 全部工作（3.5s 含下载）。

### 2.3 关键决策

| # | 决策 | 理由 |
|---|---|---|
| D1 | GUI 层用 mygo（本地 `replace`） | 任务要求基于该框架 |
| D2 | **终端用 `plugins/terminal`（Ghostty）+ 原生 UI 控制台**（方案 A，已选定） | 单二进制、无 WebView 依赖、Ghostty 级模拟质量、`Text()`/`Snapshot()` 直接服务 MCP、Mirror 直接服务多窗口 |
| D3 | 不自研 PTY 层 | 插件自带 ConPTY（已验证）；本项目 `internal/pty` 已按新方案移除 |
| D4 | SSH 用 `x/crypto/ssh` 实现 `Options.Conn` 接入，而非子进程 ssh.exe | 密码/密钥可编程认证、可存档案、MCP 可 `ssh_exec`；终端渲染质量与本地会话一致 |
| D5 | MCP 自研实现（JSON-RPC 2.0，纯 Go），stdio + Streamable HTTP | 无第三方 SDK；协议核心体量小 |
| D6 | 进程模型：GUI 进程 = 会话主机 + HTTP MCP；`adakaki mcp` 子命令 = 独立 stdio MCP（无 GUI 可用） | stdio 是 AI 客户端最通用传输；headless 会话已验证可行 |

## 3. 总体架构

```
┌──────────────────────────────────────────────────────────────────────┐
│                      ada_kaki.exe（单二进制，原生 UI）                  │
│                                                                      │
│  ┌──────────────── GUI 模式（默认）─────────────────┐                │
│  │  主窗口  ui.View(app.view)                        │                │
│  │   ├─ Sidebar：会话树（本地 / SSH 分组）+ 档案入口   │                │
│  │   ├─ TabBar：会话标签（TabsBase 自绘，含 ✕/弹出）   │                │
│  │   └─ 终端区：terminal.View(c, 选中会话的 Terminal) │                │
│  │  弹出窗口 index → terminal.View(c, Mirror终端)     │                │
│  │  对话框：SSH 连接 / 档案管理 / 新建会话             │                │
│  └───────────────────────────────────────────────────┘                │
│  HTTP 控制面 127.0.0.1:8731（Bearer token）                           │
│  MCP Streamable HTTP  /mcp  ◄── 支持 HTTP 的 AI 客户端                 │
│                                                                      │
│  ┌──────────────── mcp 模式（子命令 adakaki mcp）────┐                │
│  │  stdio JSON-RPC 2.0  ◄── AI Agent（Claude/ZCode…）│                │
│  │  独立引擎实例：headless 会话（无窗口，真实进程）    │                │
│  └───────────────────────────────────────────────────┘                │
│                                                                      │
│  ┌──────────────── core（两模式共享）────────────────┐                │
│  │  engine.SessionManager                            │                │
│  │   ├─ Session{id, kind, title, Terminal, state}    │                │
│  │   ├─ local:  terminal.Command = shell（插件自管PTY）│                │
│  │   ├─ ssh:    terminal.Conn = sshc.conn（自研SSH）  │                │
│  │   └─ 事件总线 → UI / MCP 订阅                      │                │
│  │  shells：cmd/powershell/pwsh/gitbash/custom 探测   │                │
│  │  store：config.json（档案/设置，DPAPI 加密凭据）    │                │
│  │  mcp：协议层（tools/list、tools/call…）            │                │
│  └───────────────────────────────────────────────────┘                │
└──────────────────────────────────────────────────────────────────────┘
```

**线程模型**：`terminal.Terminal` 的方法任意 goroutine 安全（内部有锁），回调（`OnTitle`/`OnExit`）在终端自己的 goroutine 上触发 → 引擎事件总线 → UI 经 `mygo.App.Post`(主线程) 刷新；MCP 工具处理在各自 goroutine，直接调引擎。

## 4. 模块设计

### 4.1 目录结构

```
ada_kaki/
├── main.go                  # 入口：默认 GUI（ui.RunGUI）；子命令 mcp / version
├── mcp_stdio.go             # 无 GUI 的 stdio MCP 子进程（headless 引擎）
├── ui/                      # 控制台界面（package ui）
│   ├── app.go               # app 状态、启动/退出、引擎事件、退出确认
│   ├── app_sessions.go      # 会话恢复/创建/SSH/关闭/重连/同屏
│   ├── app_tabs.go          # 标签与分格树模型
│   ├── app_dialogs.go       # 护栏/主机指纹问答通道
│   ├── app_windows.go       # 审计/设置窗口、外观与字体应用
│   ├── app_find.go          # 终端搜索状态
│   ├── ui_main.go           # 主窗口骨架、快捷键、内容区
│   ├── ui_header.go         # 顶栏、底部状态栏
│   ├── ui_sidebar.go        # 侧栏、折叠图标栏、会话行、Agent 卡片
│   ├── ui_tabs.go           # 标签条与标签菜单
│   ├── ui_panes.go          # 分格树、分隔线、空态、分格菜单
│   ├── ui_pane_card.go      # 分格卡片头、终端体、Agent 蒙版
│   ├── ui_dialogs.go        # 退出/护栏/主机指纹/SSH/自定义命令对话框
│   ├── ui_menus.go          # 新建/会话菜单、状态标记
│   ├── ui_find.go           # 终端搜索浮层
│   ├── ui_audit.go          # 审计窗口与筛选
│   ├── ui_popout.go         # 同屏弹出窗口
│   ├── ui_theme.go          # 柿色主题、尺寸常量
│   ├── ui_widgets.go        # 共享控件（按钮/芯片/标题条等）
│   ├── settings_view.go     # 设置窗口框架、导航、通用行
│   ├── settings_appearance.go # 外观主题、终端配色方案
│   ├── settings_terminal.go # 字体家族、字号与预览
│   ├── settings_session.go  # 默认 shell、启动恢复
│   ├── settings_mcp.go      # MCP HTTP 端点
│   ├── settings_security.go # 护栏、主机密钥、已信任主机
│   ├── settings_ai.go       # AI 操作员设置（模型/凭据/行为/护栏/预算/审计）
│   ├── ai_dock.go           # 右侧 AI 操作栏与顶部 AI 按钮
│   ├── app_ai.go            # AI 回合运行、护栏/范围适配、审计与连接探测
│   ├── tray.go              # 系统托盘与自绘菜单
│   ├── controlplane.go      # GUI 内嵌 MCP HTTP 控制面
│   ├── icons.go             # Lucide 风格单色 SVG 图标集
│   ├── branding.go          # 窗口与托盘图标
│   ├── fonts.go             # 内置字体注册
│   ├── locales.go           # 框架文案目录加载
│   ├── assets/              # icon.svg / PNG / ICO、内置字体
│   └── locales/             # zh.json
├── internal/
│   ├── engine/              # 会话引擎：Session/Manager，包装 terminal.Terminal
│   ├── shells/              # shell 探测（cmd/powershell/pwsh/gitbash/custom）与启动命令
│   ├── sshc/                # SSH 客户端：Dial/认证/交互 shell(实现 Conn 接口)/exec
│   ├── store/               # config.json 持久化；DPAPI 加密（secret_windows.go）
│   ├── guard/               # 危险命令护栏（deny/confirm 两级 + confirm_all/high_only/allow 三档）
│   ├── audit/               # MCP/AI 审计日志（JSONL 轮转）
│   ├── procjob/             # Windows Job Object 进程树清理
│   ├── tools/               # 工具统一层：会话/SSH/命令（MCP 传输与内置 Agent 共用）
│   ├── ai/                  # AI 操作员模型侧：Provider、chat/completions、凭据、token 估算
│   ├── agent/               # AI 操作员回合循环：工具调用、护栏与审计挂钩、预算
│   └── mcp/                 # MCP 协议层：JSON-RPC 2.0、Server、Tool 集
│       ├── rpc.go           #   JSON-RPC 编解码
│       ├── server.go        #   握手、传输（stdio / HTTP）
│       └── tools.go         #   12 个工具的 schema 与描述
├── docs/                    # DESIGN.md、DEVPLAN.md、ROADMAP.md、AI 规划与实施计划
└── go.mod                   # module adakaki；mygo 依赖
```

main 包只保留入口与 headless stdio；全部 GUI 状态、视图和界面资源都在 `ui` 包。界面按“窗口骨架 / 侧栏 / 标签 / 分格 / 对话框 / 菜单 / 审计 / 设置分类 / 主题与控件”分文件，单个文件控制在约 400 行内，新增界面时按同一职责落位，避免再长成单文件。

（对比 v1 移除：`internal/pty`（插件自带）、`frontend/`（原生 UI 无前端）、`internal/ansi`（`Text()` 已覆盖）。）

### 4.2 会话引擎（internal/engine）

```go
type Kind string // "local" | "ssh"

type State string // "starting" | "running" | "exited"

// Session 一个终端会话：一个 terminal.Terminal + 元数据 + 状态机。
type Session struct {
    ID      string // local_XXXX / ssh_XXXX
    Kind    Kind
    Title   string    // 本地 shell：shell 名，随后被 OSC 标题覆盖；SSH：user@host
    ShellID string    // local：cmd/powershell/pwsh/gitbash/custom
    Profile string    // ssh：来源档案名
    State   State
    ExitCode int
    // Terminal 未导出，经方法暴露 Send/Text/Snapshot/Resize/Size/Mirror
}
```

- **创建本地会话**：`terminal.New(Options{Command: shells.LaunchCmd(id, cwd), OnTitle, OnExit, Theme})`。`OnExit` → 状态机置 exited、广播事件；`OnTitle` → 更新 Title（OSC 标题来自 shell）。
- **创建 SSH 会话**：`sshc.Dial` + `NewSession` → `RequestPty`+`Shell` → 包装成 `sshConn`（实现 `io.ReadWriteCloser` + `Resize`）→ `terminal.New(Options{Conn: sshConn, ...})`。断线 → 读循环 EOF → `OnExit` 语义一致（UI 可"重连"= 新会话复用档案）。
- **会话管理**：`Manager`（RWMutex + map + 创建顺序列表）暴露 `CreateLocal/CreateSSH/Get/List/Close/Rename/Send/Text/Snapshot/Resize`。
- **主题**：`terminal.DarkTheme()`（跟随窗口深浅由插件自身处理——不设 Theme 时自动跟随）。

### 4.3 本地 Shell（internal/shells）

| ShellID | 探测 | 启动命令（作为 terminal.Options.Command） |
|---|---|---|
| `cmd` | `COMSPEC` 或 `%SystemRoot%\System32\cmd.exe` | `cmd.exe /k chcp 65001 >nul`（强制 UTF-8） |
| `powershell` | `WindowsPowerShell\v1.0\powershell.exe` | `powershell.exe -NoLogo` |
| `pwsh` | PATH 中 `pwsh.exe` | `pwsh.exe -NoLogo` |
| `gitbash` | PATH `bash.exe` / `%ProgramFiles%\Git\bin\bash.exe` / `where git` 推导 / 注册表 GitForWindows | `bash.exe -i -l` |
| `custom` | — | 用户输入的命令（可带参数） |

中文乱码对策（R2）：cmd 强制 65001；PowerShell 启动后经 `Send` 注入一行 `$OutputEncoding=[Console]::OutputEncoding=[Text.Encoding]::UTF8`（首会话可见副作用可接受，或存为 profile 脚本——实现时择优）；git-bash 原生 UTF-8。

### 4.4 SSH（internal/sshc）

- `Config{Name, Host, Port, User, AuthKind(password|key|agent), KeyPath, PassphraseEnc, PasswordEnc}`。
- 认证：`ssh.Password`（DPAPI 解密）/ `ssh.PublicKeys`(私钥+可选 passphrase) / **`agent`（OpenSSH Agent）**：Windows 命名管道 `\\.\pipe\openssh-ssh-agent`（syscall CreateFile，无 cgo、无新依赖）或 Unix `$SSH_AUTH_SOCK`。
- **HostKey TOFU（P10）**：`known_hosts` 文件（OpenSSH 兼容格式，`[host]:port keytype base64`）+ `HostKeyPolicy`：
  - 已知主机指纹匹配 → 放行；**指纹变化 → 一律拒绝**（`HostKeyChangedError` 给出两侧指纹与"忘记主机"指引——防中间人）
  - 未知主机：`prompt`（GUI 默认，指纹确认弹窗）/ `accept_new`（stdio 默认，自动存）/ `off`（不校验，设置明示"不安全"）
  - GUI 与 MCP HTTP 共享同一策略与存储；设置弹窗可"忘记"已信任主机
- 交互会话：`NewSession` → `RequestPty("xterm-256color", ...)` → `Shell()`；stdout+stderr 合并 → Conn；`Resize` → `WindowChange`。
- **断线重连（P10）**：`Session.Reconnect()`——原会话 ID/分格/标签不动，重拨存储的 `SSHSource`（GUI 侧关闭旧同屏窗口）；重连中状态 Starting 防并发重入；失败回到 Exited 可再试。MCP 工具 `reconnect_session`。
- 断线处理：读循环 `io.EOF` → 会话标记 `Exited`，UI 可重连。

### 4.5 多窗口模型

| 对象 | 说明 |
|---|---|
| 主窗口 | 常驻。Sidebar（会话树）+ TabBar + **分格区**（split panes）。每个会话的"主视图"显示在其所在分格 |
| 标签页 | 自绘 TabsBase：每标签 = 标题 + 状态点（运行绿/退出灰）+ ✕ 关闭；点击 = 把该会话换入焦点分格 |
| **分格（P8）** | 终端区是一棵二叉分格树：叶子 = 一个会话的终端，内部节点 = `ui.Split`（向右）/ `ui.SplitVertical`（向下），分隔条可拖。右键分格：向右分屏 / 向下分屏 / 关闭分格。焦点分格有强调色边框；新会话（＋新建或分屏启动）进入焦点分格 |
| **同屏弹出窗口（P7）** | "弹出同屏窗口"→ 引擎 `Session.OpenMirror()` 建第二个终端（`ResizeByConn` 模式）：帧泵 goroutine 以 33ms 周期拉 `Mirror.Frame()` → `Feed` 到镜像终端，并把镜像终端的尺寸同步为主会话尺寸；镜像终端的键盘输入经 Conn.Write 回传 `session.Send()`。主窗口标签与弹窗**实时同步、两处输入同时生效**；弹窗可随时关闭（`Mirror.Close`），会话不受影响 |
| 焦点切换 | 焦点分格 = activeID 所指会话的分格；点击分格/标签/侧栏均切换焦点 |
| 生命周期 | 主窗口关闭 = 应用退出（活跃会话 > 0 时确认）；`win.OnClosed` → 终端 `Close()`；会话关闭事件自动收拢其分格与弹窗 |

镜像管道细节：Mirror 帧 `Frame() (frame []byte, due bool)` 由第二终端的 Conn 读 goroutine 拉取（帧未就绪时阻塞/轮询 30ms），避免忙等；`Reset()` 用于弹窗重开时从头同步。

### 4.6 原生 UI 设计（ui/）

`ui` 包内引用 mygo 的 UI 包统一写作 `myui`（`import myui "github.com/egoist/mygo/ui"`），与本项目自己的 `ui` 包区分。

- 布局：`myui.Split` [ Sidebar(260) | `myui.Column` [ 标题条(= `c.TitleBar().Height`，Windows 32) | TabBar(40) | 终端区 Fill | 状态栏(28) ] ]，内容区标题条与侧栏品牌行共同承载自绘标题栏拖拽区，并与系统窗口按钮同高对齐。
- 视觉：`ui/ui_theme.go` 的 `webTheme` 在系统主题之上叠加「柿（persimmon）」强调色与中性面，`Settings.Appearance`（""/"light"/"dark"）经 `mygo.Theme.SetSource` 强制明暗（窗口按钮、菜单、终端跟随）：深色 `#0E0F12/#15171B/#24272E`、浅色 `#FBFAF7/#F2F0EA/#E1DDD4`，强调色 `#E4633C / #C24E2B`，半径 8、滚动条 8。
- Sidebar 宽度由 `myui.Split` 拖拽（夹在 200–560 DIP），品牌行右侧按钮或 `Ctrl+B` 折叠；折叠态 `sidebarRail` 是 56 DIP 的单列网格（控件 36×36、圆角 10、间距由容器的 `Padding`/`Gap` 给出），品牌标记兼任展开按钮。顶带与底带分别取 `chromeBarHeight` / `statusBarHeight`，与内容区标题条、状态栏的 1px 线保持同一行。
- Sidebar：品牌行 + plain 风格「新建会话」（无底色、悬停着色，带菜单与快捷键）+ 会话筛选 + 两节会话行（图标/标题/状态点/Agent 标记，选中行带 2px 强调边）+ 底部 Agent 活动卡与 MCP 端点行。
- TabBar：会话标签（激活项底色 + 边框、常显关闭；非激活悬停才显示）+ ＋ 快捷新建 + 分屏/搜索按钮。
- 终端区：`myui.Box` 内绘分格树；`Ctrl+F` 搜索为右上浮层（420×40），不再占整条横带。
- Agent 蒙版：半透明纱 + 卡片（bot 图标、工具名 chip、目标会话、锁定说明、审计入口），抢占焦点并拦截输入。
- 字体：`ui/assets/fonts/*.ttf` 经 `go:embed` 嵌入，启动时 `myui.RegisterFont(data, "Maple Mono NF CN")`；设置里的字体选择器用 `myui.FontFamilies()` 过滤 `Monospaced`，内置项带「（内置）」标记。
- 设置（P14）：左分类右内容的独立窗口，分类带图标分组；外观页含界面明暗（跟随系统/浅色/深色）与终端配色，标题条右侧另有太阳/月亮一键切换；改动即存，字体/字号/配色即时下发引擎。
- 菜单：每个窗口 `Context.SetMenuPresentation(myui.PopoverMenus)`，所有弹出菜单（含终端插件自带的复制/粘贴/全选/清空屏幕）都在窗口内按主题绘制；标签/侧栏的分屏等菜单仍用 `PopoverMenu`。`ui/locales/zh.json` 以 `mygo.*` 键把框架文案补成中文。
- 审计窗口：筛选（全部/成功/失败/被拦截）+ 搜索 + 列表（时间/结果/工具/命令/客户端/耗时）+ 右侧详情（元信息、命令块、护栏判定、复制命令/JSONL）。
- Toast：`myui.Toast` 提示连接失败/会话退出/复制结果。

### 4.7 AI 操作员（internal/ai · internal/agent · internal/tools）

内置 AI 是「人机同屏」的另一半：外部 Agent 走 MCP 传输，内置 Agent 走进程内调用，两者共用同一工具层与同一张规则表（[功能规划与设计](./AI-功能规划与设计.md)）。

| 组件 | 职责 |
|---|---|
| `internal/tools` | 12 个工具的实现在这里（会话/SSH/命令）。`Backend.Call(name,args)` 与 `Specs()` 同时服务 MCP 与内置 Agent，`CommandOf` 取出命令供护栏与审计 |
| `internal/ai` | Provider 抽象（本地/云端同一 `chat/completions`）、SSE 流式解析（按 `index` 合并 tool_calls 分片）、环境变量凭据（只存变量名）、token 估算 |
| `internal/agent` | 回合循环：模型 → 工具 → 结果回填 → 再问；预算可选（`ai.max_tool_calls` / `ai.session_token_limit` 手改为正数生效，0 或不设=不限制）；每次调用发事件并写审计 |
| `ui/ai_dock.go` / `ui/app_ai.go` | 右侧操作栏与顶部 AI 按钮、回合运行、护栏/工具范围适配、审计与连接探测 |
| `ui/settings_ai.go` | 六组设置：模型与推理 / 凭据与连接 / 上下文与预算 / Agent 行为 / 工具与护栏联动 / 审计与隐私 |

**状态机**：`thinking → tool → … → done | error`；预算到顶发 `turn_done{reason:"budget"|"tokens"}`，取消走 `context` 并发 `canceled`。dock 的徽标与「N 步」只数工具卡，助手消息单独成气泡。

**护栏三档**（`ai.guard_mode`）直接映射 `guard.Policy.Mode`：`confirm_all`（confirm 级问人）/ `high_only`（只问 deny 级，confirm 级放行，且先判 deny）/ `allow`（关闭护栏）。MCP 与内置 Agent 共享该策略；GUI 的人工确认通道是 `app.confirmGuard`（`win.Update` + channel，从工具协程阻塞等待）。

**与蒙版的互斥**（设计画板 08/09 的取舍）：dock 展开时不再盖整块蒙版，分格只留「AI 操作中」chip；dock 收起且 Agent 仍在运行时才走原蒙版。

**配置**：`config.json` 的 `ai` 段（缺段即默认值），键与默认值见功能规划 §7.1；设置页改动即时保存，切换护栏档位会立刻重建策略。**凭据**：直接填写的 API Key 经 DPAPI 加密存 `ai.api_key_dpapi`（优先），`ai.api_key_env` 只作环境变量兜底；旧配置把 Key 粘在环境变量框里的，启动时自动迁入加密槽。

## 5. MCP 协议设计（internal/mcp）

### 5.1 范围

MCP 规范子集：`initialize`、`notifications/initialized`、`ping`、`tools/list`、`tools/call`。仅 Tools 能力（`{tools:{listChanged:false}}`），版本协商默认 `2024-11-05`。

### 5.2 两种传输

| 传输 | 启动方式 | 场景 |
|---|---|---|
| **stdio** | `adakaki.exe mcp` | AI 客户端配置 `command: adakaki.exe, args:["mcp"]`；进程内独立引擎（无 GUI），headless 会话驱动真实进程/SSH |
| **Streamable HTTP** | GUI 启动时自动监听 `127.0.0.1:8731/mcp`（config 可改/关） | 直连操控 **GUI 可见会话**；Bearer token（`%AppData%\adakaki\token`，随机生成） |

### 5.3 工具集（v1）

| Tool | 入参 | 语义 |
|---|---|---|
| `list_shells` | – | 可用 shell |
| `list_sessions` | – | 全部会话（id/kind/title/state/exit_code） |
| `create_session` | `shell`, `cwd?`, `command?` | 新建本地会话 |
| `ssh_connect` | `profile` 或 `host,user,port,auth{password\|key\|agent}` | 新建 SSH 会话（遵循 known_hosts TOFU 策略） |
| `list_ssh_profiles` | – | 档案列表（无凭据） |
| `write_session` | `id`, `data` | 写入（AI 自带 `\r`） |
| `read_session` | `id`, `since?`, `wait_ms?`, `max_chars?` | 增量读 `Text()` 之后的输出文本（引擎为每会话维护文本快照游标：`read` 返回自游标以来终端文本的新增部分 + 新游标） |
| `screen_session` | `id` | 当前屏幕纯文本（`Text()` 截屏视图，去滚动回滚区） |
| `resize_session` | `id`, `cols`, `rows` | 调尺寸 |
| `close_session` | `id` | 关闭/断开 |
| `reconnect_session` | `id` | 重拨已退出的 SSH 会话（原 ID/分格不变） |
| `run_command` | `id`或`shell`, `command`, `timeout_ms`(默认10s), `quiet_ms`(默认800ms) | write + 轮询 Text() 直到静默窗口或超时，返回聚合输出 —— AI 高频路径 |

> 实现注：文本增量游标以"上次读取时的 `Text()` 全文长度 + 尾部锚定"做近似 diff；对全屏程序（清屏重绘）回退为返回整屏。这是 v1 务实方案，P6 依实测调优。

### 5.4 JSON-RPC 帧格式

- stdio：按行分帧（每行一个 JSON-RPC 消息）。
- HTTP：POST `/mcp` 单请求单响应（JSON 模式，无 SSE）；无 token 或非回环 → 401/拒绝。
- 工具失败 → `tools/call` 结果 `isError:true` + 文本；未初始化 → `-32001`。

### 5.5 安全护栏与审计（P11）

AI 驱动终端要可用，**每一次自动操作都必须可追责、可拦截**：

```
tools/call ──▶ ① 护栏 Guard（执行前）
                ├─ deny 命中 ──▶ 拒绝 + isError("🛡 已被安全护栏拦截：<原因>") + 审计
                ├─ confirm 命中 ─▶ GUI 弹窗人工裁决（拒绝/允许本次）
                │                   └─ 无 GUI（stdio）→ 设置决定：默认 deny
                └─ allow ──▶ ② 执行（蒙版随 tracker 出现）──▶ ③ 审计
```

- **审计日志** `internal/audit`：`%AppData%\adakaki\audit.log`，JSON Lines（时间/传输/客户端/工具/参数/会话/命令/结果/输出字节数/耗时），5MB×3 轮转；`password/passphrase/secret` 参数脱敏为 `***`；**被拦截的调用同样入账**（`guard:"denied:<原因>"`，人工放行记 `confirmed`）
- **规则库** `internal/guard`：内置 14 条（Windows+Unix，deny 级：format/diskpart/dd of=/dev/mkfs；confirm 级：rm -rf/del /s/Remove-Item -Recurse/shutdown/git push --force/git reset --hard/chmod 777/reg add HKLM…）+ config.json 自定义正则（`guard_deny`/`guard_confirm`）；`write_session` 只审"提交行"（以回车结尾的最后一行，vim 内按键不误伤）
- **三档放行强度**（设置 → AI 的 `ai.guard_mode`，MCP 与内置 Agent 共用同一策略）：`confirm_all` 每个 confirm 级匹配都问人；`high_only` 只对 deny 级匹配问一次确认，其余 confirm 级静默放行（先判 deny，避免被后面的 confirm 规则掩盖）；`allow` 关闭护栏（状态条常显警示）。GUI 的人工确认由 `app.confirmGuard` 提供（`win.Update` + channel，从工具协程阻塞等待）
- **护栏钩子**挂 MCP Server 层：`initialize` 的 clientInfo 记入审计（谁在操作）；护栏自身 panic 视为拒绝（fail-closed）；MCP HTTP 与 stdio 同一套规则，GUI 确认回调经 win.Update + channel 阻塞应答
- **GUI**：侧栏 🛡 打开审计查看窗口（最近 200 条、每 2s 自刷、右键复制命令/错误）；⚙ 设置含护栏开关与规则计数

### 5.6 Agent 操作蒙版（人机冲突防护）

AI Agent 经 MCP 工具操作会话期间，GUI 自动进入"观看模式"——人可实时看终端，但不能打扰或中断 Agent：

```
tools/call(run_command, id=X) ──▶ Adapter.begin(X, "run_command")
        │                            └─▶ session.SetAgentLock(true, tool)
        │                                  └─▶ GUI 刷新（tracker 回调里 win.Update）
        ▼
┌─ 分格 X ─────────────────────────┐
│  终端实时画面（透出）              │  ◀─ 人可以看
│  ░░░░ 半透明蒙版 RGBA(8,10,18,.55) ░░░░
│   ⏳ AI Agent 操作中               │
│   正在运行：run_command            │  ◀─ 显示当前工具名
│   可实时观看 · 输入已锁定 · 请勿打扰 │
└──────────────────────────────────┘
        │  调用返回
        └──────────▶ release() ─▶ SetAgentLock(false) ─▶ 蒙版消失
```

四层防护：

| 层 | 机制 |
|---|---|
| 1. 视觉 | 分格级半透明蒙版（`Element.Absolute()` 覆盖），Spinner + 工具名 + 提示文案；侧栏/标签状态点变 🤖 |
| 2. 输入门控 | 蒙版 `AutoFocus()` 抢占键盘焦点 + 拦截指针（终端收不到点击/按键）；同屏弹出窗口的键盘经 `Session.UserSend` —— 锁定期静默丢弃 |
| 3. 破坏性操作拦截 | 关闭会话/分格菜单项禁用并标注"（Agent 操作中）"；标签 ✕、`closeSession` 直接拒绝并 toast；退出确认框显示 "⚠ AI Agent 正在操作 N 个会话" |
| 4. Agent 通道不受影响 | MCP 写入走 `Session.Send`（绕过锁），Agent 全速操作 |

实现要点：`EngineBackend.SetAgentTracker(func(id, tool) release)`——适配器在 `write/read/screen/resize/run_command/close_session` 调用期间标记目标会话；GUI 模式注册 tracker（刷新 UI），stdio 模式无 GUI 不注册。同会话多工具并发调用各自独立标记（release 配对）。

## 6. 数据与配置

```
%AppData%\adakaki\
├── config.json   # { settings:{default_shell, font_size, mcp_http:{enabled,port},
│                 #   ssh_profiles:[{name, host, port, user, auth_kind, key_path,
│                 #                    secret_dpapi(base64)|password}],
│                 #   ai:{enabled, provider, base_url, model, reasoning, api_key_env,
│                 #       api_key_dpapi,
│                 #       context_window, max_tool_calls, session_token_limit,
│                 #       dock_open, dock_width, lock_input, auto_restore,
│                 #       tool_scope, guard_mode, audit_enabled, audit_redact,
│                 #       audit_retention_days} }
└── token         # HTTP MCP Bearer token
```

- `secret_dpapi`：Windows DPAPI（`CryptProtectData/CryptUnprotectData`，syscall）用户级加密；明文字段仅作回退并文档警示。

## 7. 错误处理与日志

- 会话启动失败/认证失败/断线：会话置 exited + 原因 → UI Toast + Sidebar 状态；MCP 工具返回 isError。
- MCP stdio 模式 **stdout 只承载 JSON-RPC**，日志全走 stderr 与文件 `%AppData%\adakaki\log.txt`（5MB×3 轮转）。

## 8. 风险与对策

| # | 风险 | 对策 |
|---|---|---|
| R1 | ghostty-vt.dll 首次运行需联网下载 | 已实测可用；发布时把 dll vendor 到 exe 旁（`LibraryPath` 优先查找），离线可用；`MYGO_GHOSTTY_VT` 可显式指定 |
| R2 | ConPTY 中文乱码 | cmd 强制 chcp 65001；PowerShell 注入 UTF8；git-bash 原生；测试覆盖中文回显 |
| R3 | Mirror 弹窗与主视图输入/尺寸竞争 | 输入统一走 `Send`（线程安全）；尺寸以视图实际尺寸为准（Mirror 终端 `ResizeByConn` 模式适配） |
| R4 | MCP 任意命令执行能力被滥用 | 回环 + token；文档警示；GUI 可关 HTTP MCP；stdio 由宿主客户端环境负责 |
| R5 | mygo 为本地开发版（replace 锁路径） | API 假设集中在 `ui/` 包与 `internal/engine`，便于随框架演进适配 |
| R6 | SSH stderr/stdout 交错 | 单读循环聚合；窗口变化经 `WindowChange` 同步 |
| R7 | `Text()` 全量读取对大回滚的性能 | MCP 读取用游标增量；`screen_session` 只取屏；必要时 P2 改用 `Snapshot()`/Mirror 精确化 |
| R8 | 内置 AI 依赖外部 provider（网络/额度/凭据） | 凭据只引用环境变量；错误在 dock 与设置页「测试连接」里可读；单轮 40 次与会话 200k token 预算防跑飞；`ai.enabled` 默认关闭 |

## 9. 测试策略

- **单元**：shells 探测与命令构造；store 读写 + DPAPI 往返；mcp 帧编解码/握手/错误码；engine 状态机（headless 真实进程）；guard 三档模式矩阵；ai 客户端（SSE/工具分片/401/取消）；agent 回合（预算/拦截/取消/审计）；tools 目录与分发；UI headless（dock 右侧布局、空态、徽章、开关持久化、端到端回合）。
- **集成**：engine 全流程（spawn→Send→Text→exit）；mcp stdio 端到端（测试内起 server，模拟客户端握手+工具调用闭环）；sshc 对本地 mock SSH server（x/crypto/ssh 服务端）。
- **GUI 验收**：手动清单（见 DEVPLAN）。
