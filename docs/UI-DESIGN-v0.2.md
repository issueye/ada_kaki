# AdaKaki 控制台 · UI 设计图（v0.2）

由 adago-design MCP 生成，**对齐当前代码现状**（不是另做一套风格）：每张画板都按
`ui_style.go` / `ui_chrome.go` / `settings_ui.go` / `icons.go` 里的实际数值还原——柿色
（persimmon）强调色、墨/纸双主题、纸条式标题栏、分格卡片、56px 收缩栏。

- 画布（在线）：<http://127.0.0.1:8788>
- 导出（离线单文件）：[adakaki-ui-design.html](adakaki-ui-design.html)（7 张画板，0 外部依赖）

## 画板清单

| # | 画板 | 对应代码 |
|---|---|---|
| 01 | 主窗口 · 深色：标题条 + 260 侧栏 + 标签条 + 双分格卡片 + Agent 蒙版 + 28 状态条 | `mainView` / `appHeader` / `sidebarPane` / `tabBar` / `paneCard` / `agentMask` / `appFooter` |
| 02 | 主窗口 · 浅色：分屏 8px 缝 + `Ctrl+F` 浮层搜索卡（钉在聚焦分格内右上） | `paneDivider` / `findOverlay` / `webTheme` 浅色分支 |
| 03 | 侧栏收缩态：56 DIP 单列网格（36×36 控件、gap 8、28×1 分隔线、激活 2px 柿色左条） | `sidebarRail` / `railAction` / `railSession` |
| 04 | 设置窗口：208 分类导航 + 外观页（主题分段 / 当前方案卡 / 312 配色方案搜索） | `settingsView` / `settingsNav` / `appearanceSettings` |
| 05 | 审计窗口：筛选（全部/成功/失败/被拦截）+ 搜索 + 六列列表 + 330 详情栏 | `auditView` / `auditFilters` / `auditRow` / `auditDetail` |
| 06 | 护栏确认弹窗 + Agent 蒙版卡 + 空状态键位引导 | `mainDialogs` / `agentMask` / `emptyPanes` |
| 07 | 视觉规范：双主题 Token、字号阶梯、尺寸与圆角、字体栈 | `webTheme` / `ui_style.go` 常量 / `monoFamily` |

## 关键规格（与代码一致）

- **强调色**：深色 `#E4633C`，浅色 `#C24E2B`；只给图标、选中态、焦点框与主按钮，
  不铺大面积。危险动作用 Danger 实心，不用感叹号堆砌。
- **底/面**：深 `#0E0F12` / `#15171B`，浅 `#FBFAF7` / `#F2F0EA`；边框 1px，靠层级与留白分区。
- **度量**：标题条 = 系统窗口按钮同高（≥ 40）· 标签条 40 / 药丸 30 / 半径 7 7 0 0 ·
  状态条 28 · 侧栏 260（拖拽 200–560，收缩 56）· 分格卡半径 12 / 头 34 / 缝 8 ·
  分格按钮 26×26 · 侧栏行 30 · 设置分类栏 208 · 审计详情栏 330。
- **字号**：对话框标题 16/Bold · 页标题 18/600 · 正文 12.5 · 行 12/500 · 辅助 11.5 ·
  状态条 10.5 · 小节标题 10.5（字距 0.6）· chip/kbd 9.5 mono。
- **字体**：界面 Inter → Noto Sans SC → system-ui；等宽 JetBrains Mono → Consolas；
  终端用内置 **Maple Mono NF CN**（含中文，OFL）。
- **图标**：单色 Lucide 风格 SVG（24 网格 / stroke 2 / round caps），颜色继承文字色，
  任意 DPI 矢量清晰；**无 emoji 当图标**。

## 设计取舍（对应 docs/UI-REVIEW.md 的 E/G/H 编号）

1. 侧栏收缩态按 **56 DIP 单列网格**重做：所有间距由容器 Padding + Gap 给，控件 36×36/半径 10，
   不再逐个 `Margin`（H1/H5）。
2. 会话行 = **一个方块**（类型图标居中 + 右下状态点），激活态是 `SurfaceHover` 底 + 2px
   柿色左条，而不是橙底加边框（H3）。
3. 分格卡头四个按钮**常显**、标题与目录让位（ellipsis，不隐藏控件），缝隙可拖可双击均分。
4. Agent 蒙版是**半透明纱 + 340 宽卡片**：单 Spinner + bot 图标 + 工具名 chip + 会话名 +
   审计入口；文案为「输入已锁定，结束后自动恢复。」（去 AI 味）。
5. 审计详情把 `denied:*` 之类判定**转成人话**，命令放 code block，底部两个复制动作。
6. 空状态用 **kbd 键位芯片**给出 `Ctrl+Shift+T` / `Ctrl+F` 的可发现性（E25'）。

## 走查结论

节点级走查 90 分（通过 3 / 警告 1 / 失败 0），唯一警告是画板里用 `var(--ui)` / `var(--mono)`
间接声明字族、机器读不到具体可商用字族——落到代码时字体栈已在上表写明（Inter / JetBrains
Mono / Maple Mono NF CN 均为可商用）。
