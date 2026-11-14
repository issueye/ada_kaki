// The AI category: the built-in AI operator's model, credentials, Agent
// behavior, guard coupling, budget and audit preferences.
//
// It writes the config.json "ai" block documented in
// docs/AI-功能规划与设计.md §7.1. The design board 10 lays the same six groups
// out in two columns; the page keeps the app's single-column settings
// rhythm (same as the other categories) so the shared row helpers and
// metrics stay identical.

package ui

import (
	"fmt"
	"os"
	"strings"

	"adakaki/internal/store"

	myui "github.com/egoist/mygo/ui"
)

// aiOption is one choice of a segmented control: the stored value and its
// label.
type aiOption struct {
	val   string
	label string
}

var (
	aiProviderOptions = []aiOption{
		{store.AIProviderLocal, "本地 commandcode"},
		{store.AIProviderCloud, "云端 open-design"},
	}
	aiReasoningOptions = []aiOption{
		{"off", "关闭"}, {"low", "低"}, {"medium", "中"}, {"high", "高"},
	}
	aiToolScopeOptions = []aiOption{
		{"all", "全部"}, {"write", "写文件"}, {"readonly", "只读"},
	}
	aiGuardModeOptions = []aiOption{
		{"confirm_all", "都需放行"}, {"high_only", "仅高危"}, {"allow", "放行"},
	}
)

// aiIndex maps a stored value to its segment index, 0 when unknown.
func aiIndex(val string, opts []aiOption) int {
	for i, o := range opts {
		if o.val == val {
			return i
		}
	}
	return 0
}

// aiValue maps a segment index back to the stored value, tolerating an
// index out of range.
func aiValue(idx int, opts []aiOption) string {
	if idx < 0 || idx >= len(opts) {
		return opts[0].val
	}
	return opts[idx].val
}

// aiLabel is the label of a stored value, falling back to the value itself.
func aiLabel(val string, opts []aiOption) string {
	for _, o := range opts {
		if o.val == val {
			return o.label
		}
	}
	return val
}

// thousands renders an integer with comma separators for read-only figures.
func thousands(n int) string {
	if n < 0 {
		return "-" + thousands(-n)
	}
	if n < 1000 {
		return fmt.Sprint(n)
	}
	return thousands(n/1000) + "," + fmt.Sprintf("%03d", n%1000)
}

func (a *app) aiSettings(c *myui.Context) {
	t := c.Theme()
	cfg := &a.store.Settings.AI

	// The widgets hold pointers to these fields, so sync them from the
	// stored values every build: an external config edit shows up here, and
	// a segment click writes both the index and the stored value.
	a.aiProviderIdx = aiIndex(cfg.Provider, aiProviderOptions)
	a.aiReasonIdx = aiIndex(cfg.Reasoning, aiReasoningOptions)
	a.aiScopeIdx = aiIndex(cfg.ToolScope, aiToolScopeOptions)
	a.aiGuardIdx = aiIndex(cfg.GuardMode, aiGuardModeOptions)
	a.aiMaxCalls = float64(cfg.MaxToolCalls)
	a.aiTokens = float64(cfg.SessionTokenLimit)

	settingsSection(c, "AI 操作员", func() {
		if myui.Checkbox(c, &cfg.Enabled, "启用内置 AI 操作员（标题条出现 AI 按钮）").
			Changed() {
			a.saveSettingsNow()
			a.refreshUI()
		}
		hint(c, "关闭时不会产生任何模型请求；MCP 端点与外部 Agent 的调用不受影响。")
	})

	settingsSection(c, "模型与推理", func() {
		fieldRow(c, "提供方", func() {
			if myui.Segmented(c, &a.aiProviderIdx, aiProviderOptions[0].label,
				aiProviderOptions[1].label).Changed() {
				cfg.Provider = aiValue(a.aiProviderIdx, aiProviderOptions)
				if cfg.Provider == store.AIProviderLocal && cfg.BaseURL == "" {
					cfg.BaseURL = store.DefaultAIBaseURL
					a.aiBaseText = cfg.BaseURL
				}
				a.saveSettingsNow()
			}
		})

		fieldRow(c, "模型", func() {
			in := myui.TextInput(c.Key("ai-model"), &a.aiModelText).
				Placeholder(store.DefaultAIModel).Grow(1).Height(28).FontSize(11.5)
			if in.Changed() || in.Submitted() {
				if v := strings.TrimSpace(a.aiModelText); v != "" {
					cfg.Model = v
					a.saveSettingsNow()
				}
			}
		})

		fieldRow(c, "推理强度", func() {
			labels := make([]string, len(aiReasoningOptions))
			for i, o := range aiReasoningOptions {
				labels[i] = o.label
			}
			if myui.Segmented(c, &a.aiReasonIdx, labels...).Changed() {
				cfg.Reasoning = aiValue(a.aiReasonIdx, aiReasoningOptions)
				a.saveSettingsNow()
			}
		})

		if cfg.Provider == store.AIProviderCloud {
			fieldRow(c, "Base URL", func() {
				in := myui.TextInput(c.Key("ai-base"), &a.aiBaseText).
					Placeholder("https://api.example.com/v1").Grow(1).Height(28).FontSize(11.5)
				if in.Changed() || in.Submitted() {
					cfg.BaseURL = strings.TrimSpace(a.aiBaseText)
					a.saveSettingsNow()
				}
			})
		}

		hint(c, "本地提供方默认直连 "+store.DefaultAIBaseURL+"，云端需填 Base URL；两种都走 OpenAI chat/completions 兼容协议。写入 config.json 的 ai.provider / ai.model / ai.reasoning。")
	})

	settingsSection(c, "凭据与连接", func() {
		fieldRow(c, "API Key 环境变量", func() {
			in := myui.TextInput(c.Key("ai-key-env"), &a.aiKeyEnvText).
				Placeholder(store.DefaultAIKeyEnv).Grow(1).Height(28).FontSize(11.5)
			if in.Changed() || in.Submitted() {
				if v := strings.TrimSpace(a.aiKeyEnvText); v != "" {
					cfg.APIKeyEnv = v
					a.saveSettingsNow()
				}
			}
		})
		myui.Row(c).FillWidth().Gap(8).AlignItems(myui.Center).Children(func() {
			name := cfg.APIKeyEnv
			state, color := "未设置", t.Warning
			if name != "" && os.Getenv(name) != "" {
				state, color = "已设置", t.Success
			}
			chip(c, name+" · "+state, color, t.Surface)
			myui.Text(c, "密钥只存变量名，不写入配置文件，也不显示明文。").
				FontSize(10.5).TextColor(t.TextMuted).Wrap().Grow(1)
			if myui.Button(c, "测试连接").Height(26).Padding(0, 10).Radius(6).
				FontSize(10.5).Clicked() {
				a.probeAI()
			}
		})
		if a.aiProbe != "" {
			probeColor := t.Warning
			if a.aiProbeOK {
				probeColor = t.Success
			}
			myui.Row(c).FillWidth().Gap(7).AlignItems(myui.Center).Children(func() {
				dot(c, probeColor, 6)
				myui.Text(c, a.aiProbe).FontSize(10.5).TextColor(probeColor).
					Wrap().Grow(1)
			})
		}
		hint(c, "写入 config.json 的 ai.api_key_env；测试连接发一次 1 token 的探测请求，不写审计。")
	})

	settingsSection(c, "上下文与预算", func() {
		fieldRow(c, "上下文窗口", func() {
			mono(c, thousands(cfg.ContextWindow)).FontSize(11).TextColor(t.Text)
			chip(c, "只读", t.TextMuted, t.Surface)
		})

		fieldRow(c, "单轮工具调用上限", func() {
			if myui.NumberInput(c, &a.aiMaxCalls, 1, 200, 1).Width(120).Changed() {
				cfg.MaxToolCalls = int(a.aiMaxCalls)
				a.saveSettingsNow()
			}
		})

		fieldRow(c, "每会话 token 上限", func() {
			if myui.NumberInput(c, &a.aiTokens, 1_000, 1_000_000, 10_000).
				Width(140).Changed() {
				cfg.SessionTokenLimit = int(a.aiTokens)
				a.saveSettingsNow()
			}
		})
		hint(c, "达到单轮上限会暂停并询问是否继续；token 到顶只提示开新会话，不打断已在跑的工具。写入 ai.max_tool_calls / ai.session_token_limit。")
	})

	settingsSection(c, "Agent 行为", func() {
		if myui.Checkbox(c, cfg.DockOpen, "AI 操作栏默认展开").Changed() {
			a.saveSettingsNow()
		}
		if myui.Checkbox(c, cfg.LockInput, "运行时锁定终端输入").Changed() {
			a.saveSettingsNow()
		}
		if myui.Checkbox(c, cfg.AutoRestore, "操作结束自动恢复输入").Changed() {
			a.saveSettingsNow()
		}
		hint(c, "锁定时拒绝人工键入，避免与 Agent 抢输入；自动恢复关掉后需手动解除，适合长时间跑批。写入 ai.dock_open / ai.lock_input / ai.auto_restore。")
	})

	settingsSection(c, "工具与护栏联动", func() {
		fieldRow(c, "允许工具", func() {
			labels := make([]string, len(aiToolScopeOptions))
			for i, o := range aiToolScopeOptions {
				labels[i] = o.label
			}
			if myui.Segmented(c, &a.aiScopeIdx, labels...).Changed() {
				cfg.ToolScope = aiValue(a.aiScopeIdx, aiToolScopeOptions)
				a.saveSettingsNow()
			}
		})

		fieldRow(c, "危险命令", func() {
			labels := make([]string, len(aiGuardModeOptions))
			for i, o := range aiGuardModeOptions {
				labels[i] = o.label
			}
			if myui.Segmented(c, &a.aiGuardIdx, labels...).Changed() {
				cfg.GuardMode = aiValue(a.aiGuardIdx, aiGuardModeOptions)
				a.saveSettingsNow()
			}
		})
		hint(c, "三档放行与「安全与护栏」页共用同一张规则表：都需放行=每次危险命令弹确认，仅高危=只拦 deny 级，放行=关闭护栏（状态条常显警示）。写入 ai.tool_scope / ai.guard_mode。")
	})

	settingsSection(c, "审计与隐私", func() {
		if myui.Checkbox(c, cfg.AuditEnabled, "记录 MCP 调用（含 AI 操作）").Changed() {
			a.saveSettingsNow()
		}
		if myui.Checkbox(c, cfg.AuditRedact, "输出脱敏（token / 密钥 / 内网主机名）").
			Changed() {
			a.saveSettingsNow()
		}
		fieldRow(c, "保留", func() {
			myui.PopoverMenuButton(c, fmt.Sprintf("%d 天", cfg.AuditRetentionDays),
				func(m *myui.Menu) {
					for _, days := range []int{7, 30, 90, 365} {
						if m.Item(fmt.Sprintf("%d 天", days)).
							Checked(cfg.AuditRetentionDays == days).Chosen() {
							cfg.AuditRetentionDays = days
							a.saveSettingsNow()
						}
					}
				}).Grow(1)
		})
		hint(c, "审计是 JSONL 轮转文件，超过保留天数在启动时清理；AI 调用记录 Transport=inapp、Client=adakaki-ai。写入 ai.audit_enabled / ai.audit_redact / ai.audit_retention_days。")
	})
}
