# adakaki MCP 功能测试报告

- **测试时间**：2026-11-13 11:00–11:15（Asia/Shanghai）
- **被测端点**：`http://127.0.0.1:8731/mcp`（GUI 内置 HTTP MCP 控制面）
- **被测进程**：`adakaki.exe`，PID 15872（StartTime 11:07:42；测试早期旧进程在 11:07 左右退出/重启，重启后 MCP 状态归零）
- **鉴权**：`Authorization: Bearer <token>`，token 位于 `%AppData%\adakaki\token`（64 位十六进制；本报告不记录具体值）
- **协议**：JSON-RPC 2.0 / MCP；`initialize` 协商结果为 `2025-03-26`，同时接受 `2024-11-05`
- **工具数**：12 个
- **测试方式**：直接用 `curl.exe` 发送原始 JSON-RPC 报文 + PowerShell 驱动；另做了一次 `adakaki mcp`（stdio）冒烟测试。**未使用** Claude Desktop / Cursor 等第三方 MCP 客户端
- **代码改动**：本次测试未改动源代码。本报告只是测试记录；测试期间新建的会话均已关闭，未操作用户已有会话

> 说明：测试过程中 GUI 侧会话数量和主题有变化（用户同时在操作界面），因此 `list_sessions` 返回的会话条数会浮动；这属于环境活动，不是 MCP 故障。

---

## 一、结果总览

| 类别 | 结论 |
|---|---|
| HTTP 鉴权（Bearer） | 通过 |
| MCP 握手 / 版本协商 / notification | 通过 |
| `tools/list` 工具目录 | 通过（12 个工具，schema 完整） |
| 本地 shell 探测与会话创建 | 通过（探测到 4 个 shell；实测创建 cmd / custom / Git Bash 会话） |
| 会话读写闭环（run/write/read/screen/resize/close） | 通过 |
| 错误处理（未知方法/坏 JSON/坏参数/未知会话） | 通过 |
| 安全护栏（deny 与 confirm） | deny 实测通过；confirm 在 headless 下按配置 deny，通过 |
| 审计与凭据脱敏 | 通过（60 条测试记录，4 条被拦截，password 已打码） |
| stdio 传输 | 冒烟通过 |
| 同会话并发调用 | **失败**（见问题 1） |
| 多客户端状态隔离 | **失败/设计缺陷**（见问题 2） |
| 超时与命令终止语义 | **有缺陷**（见问题 3、4） |

**核心结论**：握手、工具发现、会话生命周期、鉴权、审计、护栏这些主链路都能正常工作；主要风险集中在**并发正确性、超时语义和 HTTP 多客户端状态**，属于“能用但多客户端/高并发场景不可靠”的状态。

---

## 二、关键测试记录

### 2.1 鉴权

无 token / 错误 token 均返回：

```http
HTTP/1.1 401 Unauthorized
WWW-Authenticate: Bearer

{"error":"unauthorized: the token is in C:\Users\User\AppData\Roaming\adakaki\token"}
```

正确 token 返回 200。`GET /healthz` 不需要 token，返回 `{"ok":true,"sessions":N}`。

### 2.2 initialize

请求 `protocolVersion: "2025-03-26"`：

```json
{"jsonrpc":"2.0","id":1,"result":{
  "protocolVersion":"2025-03-26",
  "capabilities":{"tools":{}},
  "serverInfo":{"name":"adakaki","version":"0.1.0"},
  "instructions":"AdaKaki console: create terminal sessions ..."
}}
```

- 请求 `2025-06-18` → 服务端回退到它支持的最新版 `2025-03-26`。
- 请求 `2024-11-05` → 服务端回显 `2024-11-05`。
- 请求不带 `params` → 也能初始化，`clientName` 为空。
- `notifications/initialized` → `202 Accepted`、空响应体。
- 进程重启后、未重新 initialize 时，`tools/call` 返回 `-32001 not initialized: send initialize first`。

### 2.3 tools/list

返回 12 个工具：`list_shells`、`list_sessions`、`create_session`、`ssh_connect`、`list_ssh_profiles`、`write_session`、`read_session`、`screen_session`、`resize_session`、`close_session`、`reconnect_session`、`run_command`。每个工具都有 `name` / `description` / `inputSchema`。

### 2.4 工具实测

| 工具 | 实测结果 |
|---|---|
| `list_shells` | 返回 4 个可用 shell：cmd、powershell、pwsh、gitbash；含 path 与 found |
| `list_sessions` | 返回 id/kind/title/state/shell/has_view；运行中会话无 exit_code |
| `list_ssh_profiles` | 当前无 profile，返回 `[]` |
| `create_session`（cmd） | 成功，返回 `{"id":"local_0003","title":"Command Prompt"}` |
| `create_session`（custom） | 成功执行 `cmd /c echo custom_ok & exit`，会话进入 `exited` |
| `create_session`（未知/空 shell） | 返回 `isError`：`engine: shell nope is not available` |
| `run_command`（新建会话） | 返回 `session/output/state/timed_out`，输出包含命令回显、结果和提示符 |
| `run_command`（Git Bash） | 成功：`bash_mcp_ok` |
| `write_session` | 写入 `echo mcp_typed_ok\r` 后，`read_session` 能读到命令回显和结果 |
| `read_session` | 返回增量文本；`max_chars=200` 时返回 213 字符，末尾追加 `…[truncated]` |
| `read_session`（大输出后屏幕重置） | 返回 `[screen reset; full text follows]` + 全量文本 |
| `screen_session` | 返回当前屏幕最后 rows 行，可看到全屏程序结果 |
| `resize_session` | `100x30` 返回 `{"ok":true}`；`0x30` 返回范围错误 |
| `close_session` | 返回 `{"ok":true}`，之后 `list_sessions` 不再包含该会话 |
| `ssh_connect`（缺 host/user） | 返回 `isError`：`ssh_connect: host and user are required` |
| `ssh_connect`（未知 profile） | 返回 `isError`：`no SSH profile nope` |
| `reconnect_session` | 未测（环境无 SSH 目标/无 profile） |

### 2.5 安全护栏与审计

- `run_command` 执行 `format c: /q` → 被拦截：

  ```json
  {"content":[{"type":"text","text":"已被安全护栏拦截：格式化磁盘（...）"}],"isError":true}
  ```

- `write_session` 写入 `format c:\r`（真实回车）→ 同样被拦截，说明 write 通道也经过护栏。
- headless（stdio）模式下，confirm 级规则按默认 `deny` 处理：
  - `rm -r /tmp/mcp-test` → “递归/强制删除”拦截
  - `git reset --hard` → “丢弃全部本地改动”拦截
- 审计日志（`%AppData%\adakaki\audit.log`）记录完整字段：`time/transport/client/tool/args/session/command/ok/error/guard/duration_ms`；被拦截调用也有记录，`guard` 字段含原因。
- `ssh_connect` 参数中的 `password` 在审计里显示为 `***`，脱敏有效。
- 共统计到 60 条测试记录：57 条 HTTP、3 条 stdio；4 条被护栏拦截。

### 2.6 协议边界与 HTTP 行为

| 请求 | 结果 |
|---|---|
| 未知方法 `resources/list` | `-32601 method ... is not served` |
| 坏 JSON `{not json` | `-32700 parse error`，`id:null` |
| `arguments: []` | `-32602 bad arguments: cannot unmarshal array...` |
| 未知工具 `nope` | `isError:true`，`unknown tool "nope"`（非 JSON-RPC error） |
| 字符串 id `"abc-42"` | 原样回显 `"abc-42"` |
| JSON-RPC batch 数组 | 解析失败 `-32700`，不支持批量 |
| `GET /mcp` | `404`，文本提示只支持 POST |
| `OPTIONS /mcp` | `404`，不支持 CORS 预检 |
| `Content-Type: text/plain` + 合法 token | 仍被接受并返回 200 JSON |
| `Origin: https://evil.example` + 合法 token | 仍被接受并返回 200 |
| `Accept: text/event-stream` | 服务端仍返回 `application/json`，不提供 SSE |

### 2.7 并发、超时与输出量

**同一会话两个并发 `run_command`（local_0005）**：

- 调用 51（`echo AAA_FIRST`）的输出：

  ```
  echo AAA_FIRST
  AAA_FIRST

  C:\Users\User>echo BBB_SECOND
  BBB_SECOND

  C:\Users\User>
  ```

- 调用 52（`echo BBB_SECOND`）的输出：**空字符串**。

两个调用都“成功返回”，但命令互相串入输出、其中一方丢结果。

**`run_command` 超时**（`timeout_ms=1000`，命令为高频输出循环）：

- 返回 `timed_out=true`、`state=running`，本次已收集 **54,672 字符**；
- 1 秒后 `read_session` 仍能读到新输出（`y1441…`），`screen_session` 显示 `y20331…`；
- 说明**超时只是停止收集，不会终止命令**；只能再调用 `close_session` 结束。

**`run_command` 的 quiet 提前返回**（`ping -n 6`，`timeout_ms=1500`、`quiet_ms=500`）：

- 只返回了 2 条 Reply，`timed_out=false`；
- 其余 4 条在后续 `read_session` 中才读到；
- 即“输出间隔超过 quiet_ms”会被当成命令结束，但实际命令仍在运行。

**大输出**：`run_command` 没有 `max_chars`/字节上限；1 秒 loop 已返回 54 KB，更快的命令可能产生 MB 级响应。

### 2.8 stdio 传输（附加）

通过管道执行 `adakaki.exe mcp`：

- `initialize`、`notifications/initialized`、`tools/list`、`list_shells` 均正常返回；
- stdout 只有 JSON-RPC，stderr 为空；
- 审计记录 `transport:"stdio"`，client 为 `stdio-test`。

---

## 三、问题清单

> 严重级别说明：高 = 影响正确性/审计可信度；中 = 易踩的行为缺陷或资源风险；低 = 安全加固/协议细节；提示 = 设计取舍或未实测。

### 【高】1. 同一会话并发调用互相串扰，且没有串行化

- **现象**：两个并发 `run_command` 作用于同一会话时，一个调用看到了另一个调用的命令和输出，另一个返回空。
- **根因（代码定位）**：`internal/mcp/adapter.go` 的 `cursors` 是“按 session id 全局共享”的；`RunCommand` 在开始时重置 `cursors[id]`，随后各自循环读取 `s.Text()`；`internal/mcp/server.go` 的 `toolsCall` 没有按会话加锁/排队，HTTP 每个请求一个 goroutine。
- **影响**：MCP 客户端经常并行发起多个 tool call（或一次消息里批量调用），同一会话会出现“输出归错调用”“结果丢失”“提示符错位”。两个客户端同时读同一会话也会互相吃掉增量（后读者只能看到新内容）。
- **建议**：按会话加互斥/串行队列；读游标按客户端或连接隔离；或在并发冲突时返回明确的“busy”错误。

### 【高】2. HTTP 端点是单例 Server，`initialized` 与 `clientName` 全局共享，审计归属可被覆盖/伪造

- **现象**：
  - 进程重启后，所有客户端都必须重新 `initialize`，否则 `tools/call` 返回 `-32001`；
  - 任意客户端再次 `initialize`（例如 `clientInfo.name = "second-client"`）后，**之后所有调用**的审计 `client` 都变成 `second-client`；
  - 再发一次 `initialize` 可把名字改回来。
- **根因**：`internal/mcp/server.go` 只在 `Server` 上保存一个 `initialized bool` 和一个 `clientName`；`controlplane.go` 对每个 HTTP 请求复用同一个 `Server`，没有 `Mcp-Session-Id` 或按连接的状态。
- **影响**：多客户端（多个 Agent/多个 IDE 窗口）场景下，A 客户端初始化后 B 客户端无需初始化即可调用；审计无法区分真实客户端，且客户端名可被任意伪造，削弱“谁执行了什么”的可追溯性。
- **建议**：按 `Mcp-Session-Id` 或按请求维护初始化状态；审计身份不要只信 `clientInfo.name`；至少在多客户端时拒绝/覆盖要留下记录。

### 【中】3. `run_command` 超时只停止收集，不终止命令；没有取消工具

- **现象**：`timed_out=true` 后命令继续运行，残留输出会流入后续读取；只能 `close_session` 强制结束。
- **影响**：AI 以为“超时=命令停了”，实际后台仍在写文件/联网/编译；残留输出还会污染下一次调用。
- **建议**：超时时终止进程（或进程组），或提供显式 `cancel_command`/`kill` 工具；至少在工具说明里写清“超时不终止”。

### 【中】4. `run_command` 的 quiet 默认值会过早判定命令结束

- **现象**：`ping -n 6` 只返回 2 条 Reply 且 `timed_out=false`，剩余输出稍后才到；任何输出间隔超过 `quiet_ms`（默认 800ms）的命令都会被提前返回。
- **影响**：调用方可能误判完成；对“间歇输出”的命令（构建、下载、日志）尤其明显。
- **建议**：默认提高 quiet 窗口，或在结果里给出更明确的“可能仍在运行”标志；不要只看 quiet 就认定结束。

### 【中】5. `run_command` 输出没有大小上限

- **现象**：1 秒的高频输出命令返回 54,672 字符；接口没有 `max_chars` 参数。
- **影响**：大输出命令（`dir /s`、`type` 大文件、日志）可能造成内存暴涨、HTTP 响应过大、客户端解析慢。
- **建议**：增加输出上限并返回 `truncated` 标志；达到上限时停止收集或落盘。

### 【中】6. GUI 的 confirm 对话框只有一个槽位，并发 confirm 会永久挂起（代码观察，未实测 UI）

- **代码**：`app.go` 的 `confirmGuard` 使用单个 `a.guardCh`；如果第一个 confirm 调用还在等用户点击，第二个 confirm 到达时会把 `guardCh` 覆盖成新 channel，第一个 goroutine 的 `<-ch` 可能永远等不到答案；也没有超时。
- **影响**：多个 confirm 级调用并发时会有一个 HTTP 请求永久挂起；用户关闭窗口也可能无法唤醒。
- **建议**：confirm 请求排队或直接拒绝并发；加超时；窗口关闭时统一释放等待者。
- **备注**：本次未触发 GUI 弹窗（避免留下待确认对话框），此条仅来自代码检查。

### 【中】7. 协议版本与 HTTP 传输不完全匹配

- **现象**：服务端声明支持 `2024-11-05`，但 HTTP 只实现“单次 POST 返回 JSON”，没有 2024-11-05 时代的 HTTP+SSE（`GET /mcp` 返回 404）；也不支持 JSON-RPC batch；没有 `Mcp-Session-Id`，忽略 `MCP-Protocol-Version` 请求头；`Accept` 不生效。
- **影响**：使用旧 HTTP+SSE 传输的客户端会连不上；现代 Streamable HTTP 客户端通常可用，但协议严格性不足。
- **建议**：要么对 `2024-11-05` 不宣称 HTTP 支持，要么补齐 SSE/会话头；对 GET 返回 405 而不是 404；明确文档说明只支持单请求 JSON。

### 【低】8. `/healthz` 无鉴权

- **现象**：任何本机进程都可 `GET /healthz` 拿到 `{"ok":true,"sessions":N}`。
- **影响**：泄露会话数量，可用于探测服务是否在运行/是否有人使用；信息量不大，但端点本身没有鉴权。
- **建议**：加 token 校验，或只返回 `{"ok":true}`。

### 【低】9. 401 响应体泄露 token 文件路径

- **现象**：未认证请求的响应直接返回 `the token is in %AppData%\adakaki\token`。
- **影响**：未授权的本机调用者也能得到 token 位置提示，便于进一步尝试读取。
- **建议**：响应只保留 `unauthorized`，路径放在日志/启动提示里。

### 【低】10. 缺少 Origin 校验，且接受任意 Content-Type/Accept

- **现象**：带 `Origin: https://evil.example` 的 POST 在 token 正确时仍返回 200；`text/plain` 也被接受；`Accept` 被忽略。
- **影响**：规范建议校验 Origin 防 DNS rebinding；当前主要防线是 Bearer token。浏览器跨域携带 Authorization 会触发预检，而预检返回 404，实际可利用性低，但属于加固缺口。
- **建议**：校验 Origin allowlist（仅本机来源）；校验 Content-Type 为 `application/json`；按 Accept 返回错误或 JSON。

### 【低】11. `list_sessions` 的 `has_view` 永远为 false

- **现象**：`internal/mcp/adapter.go` 的 `sessionInfo.HasView` 只有定义、没有任何赋值；实测中包括 GUI 中由用户打开的会话也全是 `false`。
- **影响**：字段语义是“是否显示在 GUI 窗口”，但输出不可信，客户端无法据此判断会话是否已有人查看。
- **建议**：填入真实视图状态，或从 schema/输出中移除该字段。

### 【低】12. token 比较非常量时间；Windows 下 0600 不生效

- **代码**：`controlplane.go` 用 `!=` 比较完整 token；`os.WriteFile(..., 0o600)` 在 Windows 上仅依赖 ACL。
- **影响**：本地回环 + 高熵 token 下实际风险很低；token 文件权限依赖用户目录 ACL，同用户进程本就可读。
- **建议**：使用 `subtle.ConstantTimeCompare`；把 token 文件放到仅当前用户可读的目录并显式收紧 ACL（跨平台）。

### 【低/协议细节】13. 未知工具返回 `isError` 结果而非 JSON-RPC `invalid params`

- **现象**：`tools/call` 未知工具名返回 `result.isError=true` + `unknown tool`，HTTP 200。
- **影响**：多数客户端能处理，但与“未知工具属于协议错误”的实现习惯不完全一致；不影响本次主流程。
- **建议**：按所选 MCP 版本的约定统一错误形态。

### 【提示】14. 其他行为说明（非缺陷）

- `initialize` 可重复调用；`notifications/initialized` 被忽略但仍返回 202（服务端较宽容）。
- 任何持有 token 的客户端都能操作/关闭所有会话，无按客户端隔离——这是当前单 token 设计的取舍。
- 审计会记录完整命令与参数（凭据字段除外），便于追责，但也意味着命令内容会落盘。
- HTTP 返回的 `run_command` 结果包含终端回显（命令本身），不是单纯的 stdout。

---

## 四、未覆盖项

1. **真实 SSH**：无 profile、无可用目标；`ssh_connect`/`reconnect_session`/主机密钥 TOFU/known_hosts 变化拒绝均未端到端验证（只验证了参数错误路径）。
2. **GUI 人工确认（confirm 弹窗）与 Agent 蒙版**：未触发，避免在用户界面留下待确认对话框；仅通过 headless stdio 验证 confirm→deny。
3. **多客户端第三方 SDK**：未用 Claude Desktop / Cursor 等客户端做真实接入；本次是协议级原始 JSON-RPC。
4. **token 首次生成/轮换**：token 已存在；未删除 token 重启验证。
5. **审计 UI**：未打开审计窗口做筛选/复制验证；只验证了 JSONL 落盘。
6. **长时间/大负载稳定性**：未做数百并发、>1MB 请求体、长时间运行测试。
7. **不同会话之间的并发**：只测了同一会话并发；不同会话并发未发现明显问题（但未系统压测）。

---

## 五、结论与建议优先级

**当前 MCP 功能“基本可用”**：握手、12 个工具、会话生命周期、鉴权、审计、护栏都经过实测可用，stdio 也能工作。

**上线/交付前最值得先处理的三件事**：

1. **同一会话串行化**（问题 1）——多 Agent/批量 tool call 场景下会直接产生错误结果；
2. **HTTP 多客户端状态隔离与审计身份**（问题 2）——涉及“谁执行了什么”的可追溯性；
3. **超时语义**（问题 3、4）——避免 AI 误判命令已完成、后台残留。

其次是 `run_command` 输出上限（问题 5）与 confirm 并发（问题 6）。其余为安全加固与协议细节，可按版本节奏处理。

> 本次仅完成测试与问题记录，未修改任何源代码。
