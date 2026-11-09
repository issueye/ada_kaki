# AdaKaki 控制台 — 开发计划

> 依据：[设计文档](./DESIGN.md) v2.0（方案 A：plugins/terminal + 原生 UI）
> 里程碑驱动，每阶段有可验证产物。

## 阶段总览

| 阶段 | 内容 | 验收标准 | 状态 |
|---|---|---|---|
| **P0** | 可行性验证：dll 下载 + headless Ghostty 链路 | headless 测试通过（Text/Done/ExitCode） | ✅ 完成 |
| **P1** | 会话引擎 + Shell 探测 | 引擎单测通过；四种 shell 真实会话可读写 | ✅ 完成 |
| **P2** | 原生 UI 控制台 v1 | `go run .` 打开主窗口：侧栏+标签+Ghostty 终端 | ✅ 完成 |
| **P3** | 多窗口管理（弹出窗口） | 会话可弹出为独立窗口并收回 | ✅ 完成 |
| **P4** | SSH（x/crypto Conn）+ 档案 | SSH 会话交互可用；档案持久化（DPAPI） | ✅ 完成 |
| **P5** | MCP stdio | `adakaki mcp` 握手 + 工具闭环 | ✅ 完成 |
| **P6** | MCP HTTP + 设置 + 收尾 | 全量测试绿；README | ✅ 完成 |
| **P7** | 同屏弹出窗口（Mirror） | 主窗口与弹窗实时同步、两处输入生效 | ✅ 完成 |
| **P8** | 分屏（分格树） | 向右/向下分屏、拖拽分隔条、焦点边框、自动收拢 | ✅ 完成 |
| **P9** | Agent 操作蒙版 | AI 操作时人可看不可扰：蒙版+输入锁定+中断拦截 | ✅ 完成 |
| **P10** | SSH 安全与可靠性 | TOFU 指纹确认/Agent 认证/断线重连 | ✅ 完成 |
| **P11** | MCP 安全护栏与审计 | 危险命令确认/拦截 + 全量审计日志 + 查看窗口 | ✅ 完成 |
| **P12** | 体验包 | 快捷键/终端搜索/会话恢复/设置补全 | ✅ 完成 |

## P12 体验包 ✅（已完成）

- [x] 快捷键（窗口级，先于终端按键）：Ctrl+Shift+T 新建默认 shell、Ctrl+Tab / Ctrl+Shift+Tab 会话切换、Ctrl+Shift+W 关闭、Ctrl+Shift+P 弹出同屏、Ctrl+F 终端搜索
- [x] 终端内搜索：Find Bar（输入即搜/↑↓ 导航/回车下一个/✕ 或 Esc 关闭清除高亮），状态 "n / m" 来自插件 OnSearch 回调（engine.Session.SearchStatus 透传）
- [x] 会话恢复：会话增删时把 RestoreSpecs 写入 config.json（本地 shell 带工作目录与自定义命令；SSH 仅记录有档案的——无档案无法重认证），启动时逐个重建、最后一个取得焦点
- [x] 设置补全：默认 Shell 选择菜单（分屏/快捷键/恢复共用 defaultShellID）；字号保存后实时应用到全部运行中会话（terminal.SetFont）
- [x] 测试：RestoreSpecs（本地带 cwd/custom、无档案 SSH 排除、关闭后收缩）、config.json Restore 往返

## P11 MCP 安全护栏与审计 ✅（已完成）

- [x] `internal/audit`：JSON Lines 审计日志（时间/传输/客户端/工具/脱敏参数/会话/命令/结果/输出字节/耗时），5MB×3 轮转，Tail 跨轮转读取
- [x] `internal/guard`：内置 14 条规则（deny：format/diskpart/dd/mkfs；confirm：rm -rf/del /s/Remove-Item -Recurse/关机/git push --force/reset --hard/chmod 777/reg add HKLM 等）+ 自定义正则 + 三级裁决（allow/confirm/deny）+ 无 GUI 时按设置（默认 deny，fail-closed）
- [x] MCP Server 钩子：护栏先于执行（panic 视为拒绝）、initialize clientInfo 记账、password/passphrase 参数脱敏、write_session 只审提交行
- [x] GUI：危险命令确认弹窗（拒绝/允许本次，Escape=拒绝）、🛡 审计查看窗口（最近 200 条、2s 自刷、右键复制）、设置护栏开关
- [x] stdio 与 HTTP 共用规则；被拦截调用同样入账
- [x] 测试：22 条命令规则矩阵、confirm 三态（无人 deny/设置 allow/人工裁决）、自定义规则与禁用、审计记录+轮转、钩子端到端（deny 结果/审计字段/脱敏/提交行提取）


## P10 SSH 安全与可靠性 ✅（已完成）

- [x] `internal/sshc/hostkeys.go`：known_hosts 存储（OpenSSH 格式、增删查/持久化）+ `HostKeyPolicy`（TOFU：已知匹配放行、**变化拒绝**、未知按设置 prompt/accept_new/off）+ `HostKeyChangedError` 详细报错
- [x] OpenSSH Agent 认证：Windows 命名管道（syscall，无依赖）+ Unix SSH_AUTH_SOCK；`Client.Close` 一并关闭 agent 连接
- [x] GUI：首次连接指纹确认弹窗（拒绝/信任并保存，Escape 视为拒绝）；认证方式三选（密码/私钥/Agent）；设置里校验模式三档 + 已信任主机"忘记"列表
- [x] 引擎 `Session.Reconnect()`：原 ID/分格/标签保留、Starting 防重入、失败回 Exited 可再试；GUI 重连入口（会话右键）+ 成功后刷新同屏窗口
- [x] MCP：`reconnect_session` 工具（共 12 个）；HTTP MCP 共享 GUI 的 host key 策略；stdio 默认 accept_new
- [x] 测试：固定 host key 的 mock 服务器（确定性 seed）+ 公钥认证支持；策略矩阵（accept_new 存储/变化拒绝/prompt 尊重用户/insecure 不落盘）、agent 认证（TCP mock agent）、known_hosts 往返、引擎重连全流程（断线→失败→成功，ID 不变）

## P9 Agent 操作蒙版 ✅（已完成）

- [x] 引擎：`Session.SetAgentLock/AgentLock`（工具名随锁）+ `UserSend`（锁定期丢弃人工输入，`Send` 不受限）
- [x] 同屏弹出窗口键盘接入 `UserSend`（镜像端也打不进去）
- [x] MCP 适配器：`AgentTracker`——write/read/screen/resize/run_command/close_session 调用期间标记目标会话；stdio 模式无 tracker
- [x] GUI：分格半透明蒙版（Absolute 覆盖 + Spinner + 工具名 + AutoFocus 抢焦点）；状态点 🤖；关闭会话/分格拦截（菜单禁用 + toast）；退出确认显示在操作的 Agent 数
- [x] 测试：引擎锁门控（人输丢弃/Agent 输入通过/解锁恢复）、tracker begin/release 配对、HTTP 全链路（慢命令期间锁持有、响应后释放）

## P7 同屏弹出窗口 ✅（已完成）

- [x] `internal/engine/mirror.go`：`MirrorView`——镜像终端（`ResizeByConn`）+ 帧泵（33ms 拉 `Mirror.Frame()` → `Feed`，尺寸同步主会话）+ 输入回传 `Send` 的 Conn
- [x] 弹出窗口改为同屏语义：主窗口标签保留主视图，弹窗显示镜像，两处键盘输入同时生效
- [x] 会话关闭事件自动关弹窗并释放 Mirror；弹窗标题随 OSC 标题更新
- [x] 单测：帧同步（输出到达镜像）、输入转发（镜像键入到达会话）、尺寸同步、同会话多镜像

## P8 分屏 ✅（已完成）

- [x] 分格树模型：`paneNode`（叶子=会话/内部=`splitNode`）+ 焦点叶子不变式（activeID == focus.sessionID）
- [x] 递归渲染：嵌套 `ui.Split`（分隔条可拖，尺寸存于节点）；焦点分格强调色边框；空 pending 分格"启动中…"
- [x] 动作：右键分格 → 向右分屏/向下分屏（新会话进入新分格）/ 关闭分格（连会话一起关）
- [x] 标签/侧栏点击 → 会话换入焦点分格（已在树中则仅聚焦）；会话关闭事件自动收拢分格
- [x] 单测：视图/分屏/嵌套/收拢/最后分格清空（3 个）

## P0 可行性验证 ✅（已完成）

- [x] `go.mod`：module adakaki + replace mygo + x/crypto
- [x] headless 验证测试 `internal/engine/engine_test.go::TestHeadlessGhostty`
- [x] 实测：首跑自动下载 `ghostty-vt-windows-amd64.dll`（SHA-256 校验入缓存），ConPTY 启动 cmd、Ghostty 模拟、`Text()`/`Done()`/`ExitCode()` 全通过（3.5s 含下载）
- [x] 方案 A 决策并入 DESIGN.md v2.0；自研 `internal/pty` 移除

## P1 会话引擎 + Shell 探测（0.5 天）

- [ ] `internal/shells/shells.go`：探测表（cmd/powershell/pwsh/gitbash/custom）+ `LaunchCmd(id, cwd)`（chcp 65001/UTF8 注入）+ 单测
- [ ] `internal/engine/session.go`：`Session` 包装 `terminal.Terminal`（状态机 starting/running/exited、OnTitle/OnExit 回调、Send/Text/Snapshot/Resize/Size/Mirror 封装）
- [ ] `internal/engine/manager.go`：`Manager`（CreateLocal/Get/List/Close/Rename/Send/Text/…，RWMutex 并发安全）+ 事件总线（`Subscribe(func(Event))`）
- [ ] `internal/engine/manager_test.go`：真实 cmd 会话全流程（创建→Send dir→Text 含输出→Close→exit 事件）；并发 10 会话创建
- [ ] 验收：`go test ./internal/engine/ ./internal/shells/` 绿

## P2 原生 UI 控制台 v1（1 天）

- [ ] `app.go`：App 生命周期、Manager 装配、主窗口创建、退出确认（活跃会话计数）
- [ ] `ui_chrome.go`：
  - [ ] 布局骨架：Row[Sidebar | Column[TabBar | Terminal区]]（`ui.Split` 可拖）
  - [ ] Sidebar：本地/SSH 两节会话列表 + "＋新建"（`ui.MenuButton` 列 shells）+ 状态 badge
  - [ ] TabBar：`ui.TabsBase` 自绘（标题/状态色/✕ 关闭/弹出按钮）
  - [ ] 终端区：`terminal.View(c, 主视图会话.Terminal).Fill()`；切换标签换视图
  - [ ] Toast 错误提示；会话退出态视觉（灰显 + 退出码）
- [ ] 主题：跟随窗口深浅（插件默认行为）；字号设置预留
- [ ] 验收：`go run .` 可开 cmd/powershell/pwsh/gitbash 会话，vim 打开/退出（PTY 正确判据），中文无乱码

## P3 多窗口管理（0.5 天）

- [ ] `internal/engine/mirror.go`：镜像管道（Mirror.Frame 拉取 goroutine + 输入回传 Send 的 Conn 实现）
- [ ] 弹出窗口：`Popout(sessionID)` → NewWindow + 第二终端（Conn=镜像管道）；弹窗 OnClosed → Mirror.Close
- [ ] 同会话双窗口并发：输出同步、两处键盘输入均达会话；弹窗重开 `Reset()` 全量重同步
- [ ] 标签/侧栏状态联动；主窗口关闭语义（活跃会话提示）
- [ ] 验收：3 会话 + 2 弹窗场景清单通过

## P4 SSH（1 天）

- [ ] `internal/store/`：config.json 读写 + SSHProfile 模型 + DPAPI（`secret_windows.go` CryptProtectData syscall）+ 单测
- [ ] `internal/sshc/`：Dial（password/key+passphrase）、`conn` 类型（实现 io.ReadWriteCloser+Resize → WindowChange）、`Exec(host, cmd, timeout)`、断线→OnExit
- [ ] `internal/sshc/sshc_test.go`：本地 mock SSH server（x/crypto/ssh 服务端，密码+key 认证）验证 shell 字节流往返
- [ ] 引擎接入 `CreateSSH(profile, password)`；UI：SSH 连接对话框（档案 Select/新建/密码 Input/保存 Checkbox）、侧栏 SSH 分组、断线重连按钮
- [ ] 验收：对 mock server（及如有真实主机）建会话交互；档案重启保留；密码 DPAPI 落盘

## P5 MCP stdio（1 天）

- [ ] `internal/mcp/rpc.go`：JSON-RPC 2.0 行分帧编解码、错误码、initialize/版本协商
- [ ] `internal/mcp/server.go`：方法分发 + Tool 注册表（JSON Schema 入参）
- [ ] `internal/mcp/tools.go`：DESIGN §5.3 工具集 → 引擎适配接口（GUI 引擎与独立引擎同构）
- [ ] `internal/mcp/stdio.go` + `main.go` 子命令 `mcp`：无 GUI 独立运行（stdout 仅 JSON-RPC）
- [ ] `internal/mcp/mcp_test.go`：端到端——进程内 server + 模拟客户端：握手 → tools/list(11) → create_session → run_command("echo ada_kaki") → close
- [ ] 验收：ZCode/Claude 配置 `adakaki mcp` 后工具可用

## P6 MCP HTTP + 收尾（0.5 天）

- [ ] `controlplane.go`：GUI 内嵌 `127.0.0.1:8731/mcp`（JSON 模式 + Bearer token + token 文件生成/权限）
- [ ] 设置弹窗 + config.json 持久化（默认 shell、字号、MCP 开关/端口）
- [ ] 发布物：dll vendor 说明（构建脚本拷贝 dll 到 exe 旁）
- [ ] README.md（构建/运行/使用/MCP 配置示例/安全提示）；.gitignore
- [ ] 全量：`go build ./... && go vet ./... && go test ./...` 绿；手动验收清单通过

## 任务依赖图

```
P0✅ ─► P1 ─► P2 ─► P3 ─► P6
        │            └──► P4 ─┘
        └──► P5 ─────────────┘   (P5 依赖 P1 引擎；P6 依赖 P2+P5)
```

## 手动验收清单（P6 收口）

1. 新建 cmd 会话：`dir`/`chcp`(65001)/中文回显正常
2. powershell 会话：`Get-ChildItem`；pwsh 同理；git-bash：`ls -la`、vim 进/出（PTY 正确）
3. 3 个标签切换：输出不串流、滚动位置保留
4. 弹出会话 A：主窗口与弹窗内容实时同步，两处输入均生效
5. 会话 `exit`：灰显 + 退出码；重开新会话正常
6. SSH：新建档案（mock/真实）→ 连接 → `top` 正常 → 断线事件/重连
7. 重启应用：档案与设置保留；密码经 DPAPI 解密连接成功
8. MCP stdio：AI 客户端列 11 个工具 → run_command("echo ada_kaki") 返回输出
9. MCP HTTP：携带 token POST /mcp tools/list 成功；无 token 401
10. 离线启动（dll 已缓存/已 vendor）：一切正常
