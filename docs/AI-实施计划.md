# AdaKaki AI 实施计划（v0.2.1）

> 版本：v0.2（2026-11-14，A1–A6 已实现）
> 状态：A1–A6 落地、分阶段提交；自动化验收全绿，GUI 图形验收与真实 provider 连通待人工（见 §8）
> 与 [功能规划与设计](./AI-功能规划与设计.md) 配套
> 前置：P15 / P16 / P17（MCP 资源、stdio 代理、护栏编辑器与审计检索）完成后能力更完整，但本计划 **不硬依赖** P15/P16
> 执行规则沿用 [ROADMAP](./ROADMAP.md) §1：质量门全绿 → GUI 冒烟 → 手动验收 → 提交（信息引用阶段号）

---

## 1. 里程碑拆分

设计已就绪（画板 08–10 + 04 导航），实现按“能看见 → 能配置 → 能执行 → 能约束 → 能验收”的顺序拆成 6 个阶段，每阶段独立可验收、可回滚。

| 阶段 | 主题 | 预估 | 依赖 | 关键产出 |
|---|---|---|---|---|
| **A1** | 配置骨架与 AI 设置页 | 1 天 | 无 | `Settings.AI` + 6 组配置 UI（画板 10）+ 导航 AI 分类（画板 04） |
| **A2** | 模型接入与凭据 | 1 天 | A1 | `internal/ai` provider 抽象、流式客户端、环境变量凭据、测试连接 |
| **A3** | 工具执行统一层 | 1 天 | 无（可与 A2 并行） | `internal/tools` 抽出；`mcp/adapter` 变薄壳；12 工具回归绿 |
| **A4** | AI 操作栏与按钮 | 1 天 | A2 + A3 | dock 视图、顶部按钮四态、开合状态机、断点与动效（画板 08/09） |
| **A5** | 护栏、审计、预算联动 | 1 天 | A4 | `guard.Mode` 三档、审计 `inapp` 标记、单轮/会话预算 |
| **A6** | 验收、文档与开关 | 0.5 天 | A5 | U1–U12 手工清单、`DESIGN.md` 回写、`ai.enabled` 默认关闭 |

合计约 **5.5 天**（与 P0–P14 的实际节奏同量级）。

```
A1 ──► A2 ──┐
            ├──► A4 ──► A5 ──► A6
A3 ─────────┘
```

## 2. 阶段任务清单

### A1 配置骨架与 AI 设置页（1 天）

目标：不改行为的前提下，先把可配置项与页面立起来（全部默认值 + 开关关闭即等价现状）。

- [ ] `internal/store`：新增 `Settings.AI` 结构与 §7.1 全部键；旧 `config.json` 缺段取默认值；`go test` 覆盖“缺段 / 半段 / 越界值”三种解码
- [ ] `ui/settings_view.go`：`settingsGroups` 的「连接与自动化」组插入 AI（顺序 AI → MCP 端点 → 安全与护栏），图标用新增 `icoMessage`（与画板一致）
- [ ] `ui/settings_ai.go`：按画板 10 双列实现 6 组；沿用既有控件（分段 / 下拉 / 开关 / 步进 / 凭据卡）
- [ ] 校验：单轮 1–200、token 1k–1M、dock 宽 280–420；非法值不落盘、行内提示；改动即存
- [ ] 行尾配置键标注（`ai.model` / `ai.api_key_env` …）便于排障
- [ ] 手动：设置页来回切换分类不串值；重启后值保留

验收：画板 10 逐项可操作；`config.json` 出现 `ai` 段且 schema 与文档一致；关闭所有 AI 相关开关后行为与 v0.2 无异。

### A2 模型接入与凭据（1 天）⚠

目标：一个与 provider 无关的 `chat/completions` 流式客户端，本地与云端同代码路径。

- [ ] `internal/ai/provider.go`：`Provider{Name, BaseURL, Model, KeyEnv}`；`local` 默认 `http://127.0.0.1:3050/v1`
- [ ] `internal/ai/client.go`：SSE 增量解析、`context` 取消、超时（首字节 15s / 整体 300s）、错误整形（401/404/超时各一文案）
- [ ] `internal/ai/credentials.go`：只读 `os.Getenv(api_key_env)`；缺失 → 明确“未设置环境变量 X”
- [ ] `internal/ai/tokens.go`：usage 优先；缺失时按字符估算并标记 `≈`
- [ ] 设置页「测试连接」：1 token 探测、结果就地展示、不写审计
- [ ] 测试：`httptest` 假 SSE 服务器覆盖增量、超时、401、半截流

验收：本地 provider 与云端 provider 各跑通一次对话；断网与错 key 有可读错误；`go test ./internal/ai` 绿。

### A3 工具执行统一层（1 天）⚠

目标：让 MCP 传输与进程内 Agent 调用**同一份**工具实现，杜绝两份规则。

- [ ] 新建 `internal/tools`：把 `internal/mcp/adapter.go` 里的会话/SSH/命令实现迁入（`registry.go` 定义 `Tool{Name, Schema, Run}`）
- [ ] `internal/mcp/adapter.go` 改为薄壳：从 registry 取工具，`Hooks`（Guard/Audit）注入方式不变
- [ ] 工具名与 schema **零变更**：用现有 `internal/mcp` 测试 + 一次手工 `tools/list` 对比
- [ ] `internal/tools` 单测：每个工具的 happy path 用假引擎；`run_command` 超时与取消
- [ ] 记录基线：12 工具在 stdio 与 HTTP 两种传输下的响应逐字对齐（golden 文件）

验收：`go test ./internal/mcp ./internal/tools ./internal/engine` 全绿；外部 MCP 客户端行为与 v0.2 完全一致；adapter 文件行数显著下降（薄壳化）。

### A4 AI 操作栏与按钮（1 天）⚠

目标：画板 08 / 09 落地，先把“能看见”做出来。

- [ ] `ui/app_ai.go`：`DockState{closed,opening,open,closing}`、`RunState{idle,thinking,tool,running,locked,done,error,aborted}`，事件经 `mygo.App.Post` 回主线程
- [ ] `ui/ai_dock.go`：头 36 / 目标行 24 / 时间线 / 输入 56 / 元信息 24；操作卡四形态；AI 发言卡；空态
- [ ] `ui/ui_header.go`：AI 按钮四态（ghost / hover / accent + 徽标 / 无 Agent），26 高、徽标 14、`>99 → 99+`
- [ ] `ui/ui_main.go`：dock 挂载；`Ctrl + Alt + A`；宽度拖拽 280–420（双击复位 340）；<1280 覆盖式 + 左阴影
- [ ] 动效：160ms `cubic-bezier(.2,.8,.2,1)`，内容延迟 80ms 淡入；`Esc` 只收焦点
- [ ] `ui/ui_pane_card.go`：dock 展开时撤蒙版、分格头改「AI 操作中」chip；收起且运行中恢复蒙版
- [ ] `ui/ui_theme.go`：dock 尺寸常量集中定义（禁止散落魔法数字）
- [ ] 持久化：`ai.dock_open` / `ai.dock_width` 写回 config，重启还原

验收：U1–U3 通过（含断点与快捷键）；画板 08 的尺寸逐项对齐；dock 收起后主窗口与 v0.2 像素级一致。

### A5 护栏、审计、预算联动（1 天）⚠

目标：让 AI 的每一次调用都受同一套规则约束，且可追溯。

- [ ] `internal/guard`：`Policy` 增加 `Mode`（`confirm_all｜high_only｜allow`）+ `ApplyMode()`；不改 `Check` 语义
- [ ] 设置页三档开关即时重建 Policy；`allow` 档在状态条常显警示（琥珀）
- [ ] `ui/app_dialogs.go`：护栏确认对话框复用现有实现；Agent 协程阻塞等待，取消视为 `deny`
- [ ] 审计：进程内调用 `Transport="inapp"`、`Client="adakaki-ai"`；`Args` 内嵌 `turn`；脱敏规则覆盖 token 与密钥
- [ ] 深链：操作卡「查看审计」按 `(time, tool, session)` 定位审计窗口行
- [ ] 预算：`max_tool_calls`（40）到顶暂停 + 「继续」追加；`session_token_limit`（200k）到顶提示；到顶不打断在跑的工具
- [ ] 测试：三档放行矩阵（6 个命令 × 3 档）、预算边界（39/40/41）、审计 `inapp` 记录

验收：U5–U9 通过；审计窗口能看到 AI 调用且与 MCP 调用可区分；`denied` 原因与护栏对话框一致。

### A6 验收、文档与开关（0.5 天）

- [ ] `ai.enabled` 默认 `false`：关闭时标题条无按钮、dock 不挂载、MCP 服务端照常；开启后完整链路可用
- [ ] 跑完整 U1–U12 手工清单并记录结果
- [ ] `docs/DESIGN.md` 回写：新增 `internal/ai|agent|tools` 三包、`ui/ai_dock.go` 等文件、dock 与蒙版的互斥规则
- [ ] `README.md` 功能清单补一条「内置 AI 操作员（可关）」
- [ ] `docs/ROADMAP.md` 插入本阶段（见 §5）
- [ ] 提交信息引用阶段号（A1–A6）

## 3. 任务看板（可拆给并行开发）

| ID | 任务 | 文件 | 依赖 | 估时 |
|---|---|---|---|---|
| T1 | `Settings.AI` 结构与兼容解码 | `internal/store/store.go` | — | 0.2 天 |
| T2 | AI 设置页 6 组 UI | `ui/settings_ai.go` | T1 | 0.5 天 |
| T3 | 导航注册与图标 | `ui/settings_view.go`、`ui/icons.go` | T1 | 0.1 天 |
| T4 | provider 抽象与凭据 | `internal/ai/provider.go`、`credentials.go` | — | 0.3 天 |
| T5 | 流式客户端与错误整形 | `internal/ai/client.go` | T4 | 0.5 天 |
| T6 | token 计量 | `internal/ai/tokens.go` | T5 | 0.15 天 |
| T7 | 工具注册表抽取 | `internal/tools/*` | — | 0.6 天 |
| T8 | adapter 薄壳化 + 回归 | `internal/mcp/adapter.go` | T7 | 0.3 天 |
| T9 | dock 视图（画板 08） | `ui/ai_dock.go` | T5、T7 | 0.6 天 |
| T10 | AI 按钮四态（画板 09） | `ui/ui_header.go` | T9 | 0.2 天 |
| T11 | 开合/断点/动效/拖拽 | `ui/ui_main.go`、`ui/ui_theme.go` | T9 | 0.4 天 |
| T12 | 蒙版与 chip 互斥 | `ui/ui_pane_card.go` | T9 | 0.2 天 |
| T13 | guard Mode 与映射 | `internal/guard/guard.go` | — | 0.2 天 |
| T14 | 审计 inapp + 脱敏 + 深链 | `internal/agent/loop.go`、`ui/ui_audit.go` | T9、T13 | 0.4 天 |
| T15 | 预算（单轮/会话） | `internal/agent/budget.go` | T14 | 0.3 天 |
| T16 | 事件流与状态机接线 | `ui/app_ai.go` | T9 | 0.3 天 |
| T17 | 手工清单 U1–U12 | — | 全部 | 0.3 天 |
| T18 | DESIGN/README/ROADMAP 回写 | `docs/*` | 全部 | 0.2 天 |

## 4. 测试矩阵

| 层 | 对象 | 方式 |
|---|---|---|
| 单元 | 配置解码、guard Mode 映射、预算计数、token 估算、SSE 解析 | `go test`，表驱动 |
| 集成 | 假 provider（httptest SSE）+ 假引擎：一轮含 3 次工具调用 | `internal/agent` 测试 |
| 回归 | 12 工具 × stdio/HTTP 的 golden 响应 | 复用 `internal/mcp` 装置 |
| GUI 冒烟 | 开合、断点、锁定/恢复、护栏对话框 | 手工 + 现有 automation（若 P24 已就绪） |
| 设计一致性 | 画板 08/09/10 尺寸与状态 | 截图比对 + 走查报告 |

## 5. 与 ROADMAP 的衔接（建议）

ROADMAP 目前 v0.2 里程碑为 P15–P17、之后是 v0.3 的 P18–P20。本计划建议作为 **v0.2 的追加阶段**插入在 P17 之后、P18 之前，编号 **P26–P31**（A1–A6 各对应一个），理由是：

- P17（护栏编辑器 + 审计检索）与 A5 共用规则表与审计窗口 → 先做 P17 能省掉 A5 一半工作；
- 本计划不依赖 P16（stdio 代理），也不阻塞 P18（跨平台）——若排期紧张，A3/A4 可与 P18 并行；
- 发布口径建议：`v0.2.1`（AI 操作员）作为 v0.2 的补丁里程碑，含 `ai.enabled` 开关，默认关闭，可在 v0.3 默认打开。

待评审确认后再改 ROADMAP §2 与 §8 状态表（本次未改，避免与既有编号冲突）。

## 6. 回滚与降级

1. **纵深开关**：`ai.enabled=false` 即完全回到 v0.2 行为（按钮、dock、agent 循环、模型请求全部不启用）。
2. **单点降级**：provider 不可用时 dock 仍可显示历史；「无 Agent」态允许用户只看审计。
3. **数据兼容**：`config.json` 的 `ai` 段缺失即默认值，删除该段等价关闭；审计文件不需要迁移。

## 7. 待决策项（开工前需确认）

| # | 问题 | 建议 | 影响 |
|---|---|---|---|
| Q1 | `tool_scope` 三档的精确语义 | `all`：12 工具全开；`write`：允许 `run_command` / `write_session`，禁 `create/close/reconnect/ssh_connect`；`readonly`：只允许 `list/read/screen` | A5 默认策略 |
| Q2 | 一轮是否允许跨会话 | 只允许目标会话；切换会话即开新一轮 | A4 状态机 |
| Q3 | 「继续」是否继承上一轮上下文 | 继承（同一 turn 链），但工具计数重置 | A5 预算 |
| Q4 | 流式渲染粒度 | 段落级（`\n\n` 或 80 字符）而非逐 token | A4 重排成本 |
| Q5 | 是否在 v0.2.1 就支持外部 MCP 与内置 Agent 同会话并发 | 不支持，串行（MCP 调用仍独立） | A1–A5 范围 |

---

## 8. 实施进度与验收状态（2026-11-14）

### 8.1 阶段进度

| 阶段 | 状态 | 提交 | 证据 |
|---|---|---|---|
| A1 配置骨架与 AI 设置页 | ✅ | `AI A1` | `internal/store` 6 项单测；设置页六组；导航 AI 分类 |
| A2 模型接入与凭据 | ✅ | `AI A2` | `internal/ai` 11 项单测（SSE/工具分片/401/取消/估算）；测试连接 |
| A3 工具统一层 | ✅ | `AI A3` | `internal/tools` 契约测试；12 工具目录金标准；mcp 既有测试跑在抽出的实现上 |
| A4 AI 操作栏与顶部按钮 | ✅ | `AI A4` | headless 渲染测试（右侧布局/空态/徽章/禁用/持久化） |
| A5 护栏/审计/预算联动 | ✅ | `AI A5` 两笔 | guard 三档矩阵；agent 6 项回合测试；UI 端到端回合（模型→工具→审计→dock） |
| A6 验收与文档回写 | ✅ | `AI A6` | DESIGN/README/ROADMAP 回写；两份 AI 文档入库；`go build/vet/test` 全绿 |

### 8.2 U1–U12 验收状态

| # | 用例 | 状态 | 说明 |
|---|---|---|---|
| U1 | 点 AI 按钮展开 | ⚠ 部分 | 开合状态与持久化有单测；160ms 动效与视觉待人工 |
| U2 | `Ctrl+Alt+A` | ⚠ 部分 | 快捷键已接线，待人工按键验收 |
| U3 | 窗口缩到 1200 | ✗ 未实现 | 覆盖式断点尚未做（当前始终挤压式） |
| U4 | 提交“跑一遍测试” | ✅ | UI 端到端回合测试（fake provider + 真实工具层 + 审计） |
| U5 | 触发 `rm -rf` | ⚠ 部分 | 护栏三档与拒绝路径有单测；GUI 弹窗放行待人工 |
| U6 | 运行中锁定输入 | ⚠ 部分 | 锁由共享 backend 的 tracker 打上，dock 展开时显示 chip；待人工确认观感 |
| U7 | 关掉 `lock_input` | ✗ 未实现 | 配置已存，但尚未改变引擎锁行为（见 §9 偏差 6） |
| U8 | 单轮 40 步 | ✅ | agent 预算测试：第 41 次不执行、`turn_done{reason:"budget"}` |
| U9 | token 到 200k | ✅ | agent token 上限测试：不再执行后续工具并提示 |
| U10 | 断网 / 错 key | ⚠ 部分 | 缺 key、401、取消、坏流均有单测；真实 provider 断网待人工 |
| U11 | 关闭 `ai.enabled` | ✅ | dock 不渲染的 headless 测试 + MCP 全量回归 |
| U12 | 外部 MCP 客户端 | ✅ | `internal/mcp`/`ui` 既有端到端测试全绿 |

### 8.3 与计划的偏差（落地时记录）

1. 设置页为**单列**（画板 10 是双列），与其它设置分类保持同一节奏；待视觉走查后可改。
2. 模型是**文本输入**而非下拉（当前没有模型目录接口）。
3. UI 回合使用**非流式**调用（`ai` 客户端已支持 SSE，接流式留到下一版；事件模型已按增量设计）。
4. `<1280` 的覆盖式断点未实现（当前是挤压式）。
5. dock 头部的「更多」菜单未做。
6. `ai.lock_input` / `ai.auto_restore` 只存配置，引擎锁仍由 tracker 一次性开关；后续按设置决定是否打锁。
7. `ai.audit_retention_days` 只存配置，启动时清理未实现。
8. 会话 token 计数在内存中，重启归零（未持久化）。

## 9. 变更记录

| 日期 | 版本 | 变更 |
|---|---|---|
| 2026-11-14 | v0.1 | 初稿：A1–A6 阶段、T1–T18 看板、测试矩阵、ROADMAP 衔接建议、5 项待决策 |
| 2026-11-14 | v0.2 | A1–A6 实施完成（分阶段提交）；补 §8 进度、U 状态与 8 项偏差；测试矩阵已落实 |
| 2026-11-14 | v0.3 | 凭据支持**直接保存**：`ai.api_key_dpapi`（DPAPI 加密，优先于环境变量），旧的「环境变量框里粘 Key」配置启动时自动迁移；设置页改为 API Key 直接填写 + 可选环境变量兜底 |
