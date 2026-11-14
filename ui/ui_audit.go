// The audit window: the filters, the list of MCP calls and the
// detail panel, with the filtering and copy helpers.

package ui

import (
	"adakaki/internal/audit"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
	myui "github.com/egoist/mygo/ui"
)

// auditView is the audit window: the filters over the calls, the list of
// what the agent did, and the details of the chosen call beside it.
func (a *app) auditView(c *myui.Context) {
	webTheme(c)
	c.SetMenuPresentation(myui.PopoverMenus)
	t := c.Theme()
	var entries []audit.Entry
	total := int64(0)
	if a.auditLog != nil {
		entries, _ = a.auditLog.Tail(200)
		total = a.auditLog.Written()
	}
	// Tail reads newest first, as a log reads
	shown := a.filterAudit(entries)
	sel := a.selectedAudit(shown)

	myui.Column(c).Fill().Background(t.Background).Children(func() {
		a.auditTitleStrip(c, total, len(shown))
		myui.Divider(c)
		a.auditFilters(c, len(shown), len(entries))
		myui.Divider(c)
		myui.Row(c).Fill().Children(func() {
			myui.Column(c).Fill().Children(func() {
				a.auditHeadRow(c)
				if len(shown) == 0 {
					myui.Column(c).Fill().Center().Gap(8).Children(func() {
						myui.Icon(c, icoHistory).FontSize(26).TextColor(t.TextMuted)
						myui.Text(c, a.auditEmptyText()).FontSize(12).TextColor(t.TextMuted)
					})
					return
				}
				myui.Scroll(c).Fill().Children(func() {
					myui.Column(c).FillWidth().Children(func() {
						for _, e := range shown {
							a.auditRow(c, e, sel)
						}
					})
				})
			})
			myui.Box(c).Width(1).FillHeight().Background(t.Border)
			a.auditDetail(c, sel)
		})
	})
	// The window repaints with fresh entries on its own.
	c.After(2 * time.Second)
}

// auditTitleStrip is the window's own title bar.
func (a *app) auditTitleStrip(c *myui.Context, total int64, shown int) {
	t := c.Theme()
	titleStrip(c, "MCP 审计日志", func() {
		chip(c, fmt.Sprintf("%d 条", total), t.TextMuted, t.Background)
		myui.Text(c, fmt.Sprintf("当前显示 %d 条", shown)).
			FontSize(10.5).TextColor(t.TextMuted)
	})
}

// auditFilters is the row of result filters, the search field and the
// copy-all action.
func (a *app) auditFilters(c *myui.Context, shown, loaded int) {
	t := c.Theme()
	kinds := []struct{ id, label string }{
		{"all", "全部"}, {"ok", "成功"}, {"error", "失败"}, {"denied", "被拦截"},
	}
	myui.Row(c).FillWidth().Height(48).Padding(0, 16).Gap(8).
		AlignItems(myui.Center).Background(t.Background).Children(func() {
		for _, k := range kinds {
			active := a.auditFilter == k.id || (a.auditFilter == "" && k.id == "all")
			b := myui.Button(c, k.label).Height(26).Padding(0, 10).Radius(6).
				FontSize(10.5).Shrink(0)
			if active {
				b = b.Background(t.Surface).Border(1, t.Border).FontWeight(500)
			} else {
				b = b.Background(myui.Transparent).BorderWidth(0)
			}
			if b.Clicked() {
				a.auditFilter = k.id
			}
		}
		myui.Row(c).Width(220).Height(26).Padding(0, 8).Gap(6).Radius(6).
			Background(t.Background).Border(1, t.Border).
			AlignItems(myui.Center).Children(func() {
			myui.Icon(c, icoSearch).FontSize(11).TextColor(t.TextMuted)
			myui.TextInput(c.Key("audit-search"), &a.auditQuery).Grow(1).Height(22).
				FontSize(10.5).BorderWidth(0).Background(myui.Transparent).
				Placeholder("搜索命令、客户端").PlaceholderColor(t.TextMuted)
		})
		myui.Spacer(c)
		myui.Text(c, fmt.Sprintf("%d / %d", shown, loaded)).
			Font(monoFamily()).FontSize(10).TextColor(t.TextMuted)
		if myui.Button(c, "复制筛选结果").Height(26).Padding(0, 9).Radius(6).
			FontSize(10.5).Background(t.Surface).Border(1, t.Border).
			Tooltip("把当前筛选结果以 JSONL 复制到剪贴板").
			Clicked() {
			a.copyAudit()
		}
	})
}

// auditHeadRow is the list's column captions, aligned with the rows.
func (a *app) auditHeadRow(c *myui.Context) {
	t := c.Theme()
	myui.Row(c).FillWidth().Height(30).Padding(0, 16).Gap(10).
		Background(t.Surface).AlignItems(myui.Center).Children(func() {
		cols := []struct {
			label string
			w     float32
			grow  bool
		}{
			{"时间", 62, false}, {"", 14, false}, {"工具", 124, false},
			{"命令", 0, true}, {"客户端", 86, false}, {"耗时", 46, false},
		}
		for _, col := range cols {
			txt := myui.Text(c, col.label).FontSize(9.5).FontWeight(500).
				TextColor(t.TextMuted)
			if col.grow {
				txt.Grow(1)
			} else {
				txt.Width(col.w)
			}
		}
	})
}

// auditRow is one call: time, outcome, tool, command, client, duration.
func (a *app) auditRow(c *myui.Context, e audit.Entry, sel *audit.Entry) {
	t := c.Theme()
	key := auditKey(e)
	selected := sel != nil && auditKey(*sel) == key

	ico, color := icoCheck, t.Success
	switch {
	case e.Guard != "" && strings.HasPrefix(e.Guard, "denied"):
		ico, color = icoShieldX, t.Danger
	case !e.OK:
		ico, color = icoCircleX, t.Danger
	case e.Guard == "confirmed":
		ico, color = icoShield, t.Warning
	}

	row := myui.Row(c.Key(key)).FillWidth().Height(34).Padding(0, 16).Gap(10).
		AlignItems(myui.Center)
	switch {
	case selected:
		row = row.Background(t.SurfaceHover)
	case row.Hovered():
		row = row.Background(t.Surface)
	}
	row.Children(func() {
		mono(c, e.Time.Format("15:04:05")).FontSize(10.5).
			TextColor(t.TextMuted).Width(62)
		myui.Icon(c, ico).FontSize(13).TextColor(color)
		mono(c, e.Tool).FontSize(10.5).TextColor(t.Text).Width(124).
			SingleLine().Ellipsis("…")
		cmd := e.Command
		if cmd == "" {
			cmd = "—"
		}
		mono(c, cmd).FontSize(10.5).TextColor(t.TextMuted).Grow(1).
			SingleLine().Ellipsis("…")
		myui.Text(c, orDash(e.Client)).FontSize(10).TextColor(t.TextMuted).Width(86).
			SingleLine().Ellipsis("…")
		mono(c, fmt.Sprintf("%dms", e.DurationMS)).FontSize(10).
			TextColor(t.TextMuted).Width(46)
	})
	row.PopoverMenu(func(m *myui.Menu) {
		if e.Command != "" {
			if m.Item("复制命令").Chosen() {
				mygo.Clipboard.WriteText(e.Command)
			}
		}
		if e.Err != "" {
			if m.Item("复制错误").Chosen() {
				mygo.Clipboard.WriteText(e.Err)
			}
		}
	})
	if row.Clicked() {
		a.auditSel = key
	}
}

// auditDetail shows everything the list had to shorten about one call.
func (a *app) auditDetail(c *myui.Context, e *audit.Entry) {
	t := c.Theme()
	myui.Column(c).Width(330).FillHeight().Background(t.Surface).
		Padding(16).Gap(12).Children(func() {
		myui.Row(c).FillWidth().Gap(8).AlignItems(myui.Center).Children(func() {
			myui.Text(c, "调用详情").FontSize(12.5).FontWeight(600)
			myui.Spacer(c)
			if e != nil {
				if myui.Button(c, "").Label("复制 JSONL").Padding(4).Radius(5).
					Background(myui.Transparent).BorderWidth(0).Tooltip("复制这条记录").Children(func() {
					myui.Icon(c, icoCopy).FontSize(12).TextColor(t.TextMuted)
				}).Clicked() {
					if line, err := json.Marshal(e); err == nil {
						mygo.Clipboard.WriteText(string(line))
					}
				}
			}
		})
		if e == nil {
			myui.Column(c).Fill().Center().Gap(8).Children(func() {
				myui.Icon(c, icoShield).FontSize(22).TextColor(t.TextMuted)
				myui.Text(c, "选择一条记录查看详情").FontSize(11.5).TextColor(t.TextMuted)
			})
			return
		}

		outcome, color := "成功", t.Success
		switch {
		case e.Guard != "" && strings.HasPrefix(e.Guard, "denied"):
			outcome, color = "被护栏拦截", t.Danger
		case !e.OK:
			outcome, color = "执行失败", t.Danger
		case e.Guard == "confirmed":
			outcome, color = "人工确认后放行", t.Warning
		}
		myui.Row(c).Gap(7).AlignItems(myui.Center).Children(func() {
			dot(c, color, 6)
			myui.Text(c, outcome).FontSize(11.5).FontWeight(500).TextColor(color)
		})

		metaRow(c, "工具", e.Tool, true)
		metaRow(c, "客户端", orDash(e.Client), false)
		metaRow(c, "目标会话", orDash(e.Session), false)
		metaRow(c, "时间", e.Time.Format("01-02 15:04:05"), true)
		metaRow(c, "耗时", fmt.Sprintf("%d ms", e.DurationMS), true)
		metaRow(c, "传输", orDash(e.Transport), false)

		myui.Divider(c)
		sectionLabel(c, "命令")
		codeBlock(c, func() {
			cmd := e.Command
			if cmd == "" {
				cmd = "（未执行命令）"
			}
			mono(c, cmd).FontSize(10.5).Wrap()
			if e.OutputBytes > 0 {
				mono(c, fmt.Sprintf("输出 %d 字节", e.OutputBytes)).
					FontSize(9.5).TextColor(t.TextMuted)
			}
		})
		if e.Err != "" {
			sectionLabel(c, "错误")
			myui.Text(c, firstLine(e.Err)).FontSize(10.5).TextColor(t.Danger).Wrap()
		}
		if e.Guard != "" {
			sectionLabel(c, "护栏")
			myui.Text(c, guardText(e.Guard)).FontSize(10.5).TextColor(t.Warning).Wrap()
		}

		myui.Spacer(c)
		myui.Row(c).FillWidth().Gap(8).Children(func() {
			if myui.Button(c, "复制命令").Height(30).Padding(0, 12).Radius(6).
				FontSize(11).Grow(1).Background(t.Accent.Alpha(0.14)).
				BorderWidth(0).TextColor(t.Accent).Clicked() && e.Command != "" {
				mygo.Clipboard.WriteText(e.Command)
			}
			if myui.Button(c, "复制 JSONL").Height(30).Padding(0, 12).Radius(6).
				FontSize(11).Grow(1).Background(myui.Transparent).
				Border(1, t.Border).Clicked() {
				if line, err := json.Marshal(e); err == nil {
					mygo.Clipboard.WriteText(string(line))
				}
			}
		})
	})
}

// metaRow is a label and its value in the audit detail.
func metaRow(c *myui.Context, label, value string, code bool) {
	t := c.Theme()
	myui.Row(c).FillWidth().Gap(8).AlignItems(myui.Center).Children(func() {
		myui.Text(c, label).FontSize(10.5).TextColor(t.TextMuted).Width(56)
		txt := myui.Text(c, value).FontSize(11).TextColor(t.Text).
			SingleLine().Ellipsis("…").Grow(1)
		if code {
			txt.Font(monoFamily())
		}
	})
}

// filterAudit narrows the trail by outcome and by a search over tool,
// command and client.
func (a *app) filterAudit(entries []audit.Entry) []audit.Entry {
	q := strings.ToLower(strings.TrimSpace(a.auditQuery))
	out := make([]audit.Entry, 0, len(entries))
	for _, e := range entries {
		denied := strings.HasPrefix(e.Guard, "denied")
		switch a.auditFilter {
		case "ok":
			if !e.OK || denied {
				continue
			}
		case "error":
			if e.OK {
				continue
			}
		case "denied":
			if !denied {
				continue
			}
		}
		if q != "" {
			hay := strings.ToLower(e.Tool + " " + e.Command + " " + e.Client + " " + e.Err)
			if !strings.Contains(hay, q) {
				continue
			}
		}
		out = append(out, e)
	}
	return out
}

// selectedAudit is the entry the detail panel shows: the chosen one, or
// the newest of the filtered list.
func (a *app) selectedAudit(entries []audit.Entry) *audit.Entry {
	if len(entries) == 0 {
		return nil
	}
	for i := range entries {
		if auditKey(entries[i]) == a.auditSel {
			return &entries[i]
		}
	}
	return &entries[0]
}

// auditKey names one call for selection, stable across refreshes.
func auditKey(e audit.Entry) string {
	return fmt.Sprintf("%d-%s", e.Time.UnixNano(), e.Tool)
}

// guardText reads a guard verdict in words.
func guardText(g string) string {
	switch {
	case g == "":
		return ""
	case g == "confirmed":
		return "命中规则，人工确认后放行"
	case strings.HasPrefix(g, "denied:"):
		return "已拦截：" + strings.TrimPrefix(g, "denied:")
	default:
		return g
	}
}

// auditEmptyText explains an empty list.
func (a *app) auditEmptyText() string {
	if (a.auditFilter != "" && a.auditFilter != "all") || strings.TrimSpace(a.auditQuery) != "" {
		return "没有符合筛选条件的调用"
	}
	return "还没有 MCP 调用记录"
}

// copyAudit puts the filtered trail on the clipboard as JSON lines.
func (a *app) copyAudit() {
	if a.auditLog == nil {
		return
	}
	entries, _ := a.auditLog.Tail(200)
	shown := a.filterAudit(entries)
	var b strings.Builder
	for _, e := range shown {
		if line, err := json.Marshal(e); err == nil {
			b.Write(line)
			b.WriteByte('\n')
		}
	}
	mygo.Clipboard.WriteText(b.String())
	a.pendingToast = fmt.Sprintf("已复制 %d 条审计记录", len(shown))
	if a.auditWin != nil {
		a.auditWin.Invalidate()
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
