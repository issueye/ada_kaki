// The console's state and lifecycle: the app struct, the split and
// workspace model, startup, engine events and the quit path.

package ui

import (
	"adakaki/internal/ai"
	"adakaki/internal/audit"
	"adakaki/internal/engine"
	"adakaki/internal/guard"
	"adakaki/internal/procjob"
	"adakaki/internal/shells"
	"adakaki/internal/sshc"
	"adakaki/internal/store"
	"adakaki/internal/tools"
	"context"
	"fmt"
	"log"
	"runtime"
	"sync"
	"time"

	"adakaki/internal/agent"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	myui "github.com/egoist/mygo/ui"
)

// app is the whole console: the session engine, the main window and the
// popped-out windows.
type app struct {
	mgr    *engine.Manager
	shells []shells.Shell

	win *mygo.Window // the main window

	sidebarWidth     float32
	sidebarCollapsed bool   // the session column folded away
	activeID         string // the session the main window shows

	popouts map[string]*mygo.Window // session ID → popped-out window

	// configuration, persisted by internal/store
	store *store.Config

	// dialog state, for myui.Modal
	showQuit   bool
	quitArmed  bool
	quitting   bool // the app is closing all of its windows
	showCustom bool
	customCmd  string

	// the SSH connect dialog
	showSSH    bool
	sshProfile string // the profile a menu choice filled the fields from
	sshHost    string
	sshPort    string
	sshUser    string
	sshAuth    string // "password" | "key" | "agent"
	sshKeyPath string
	sshPass    string
	sshSave    bool
	sshSaveAs  string

	// the host key dialog, for a first meeting with an SSH host
	showHostKey bool
	hkHost      string
	hkPort      int
	hkFP        string
	hkCh        chan bool

	// trusted SSH host keys, and the dialog state for settings
	hostKeys *sshc.HostKeyStore

	// the MCP rails: the audit trail and the command guardrail
	auditLog *audit.Log
	guard    *guard.Policy

	// the guardrail confirm dialog
	showGuardAsk bool
	guardReq     *guard.Request
	guardRule    *guard.Rule
	guardCh      chan bool

	// the audit viewer window
	auditWin *mygo.Window

	// the notification area icon, or nil when the system offers none,
	// and its drawn menu while that shows; trayClosed damps a right click
	// that closed the menu as it deactivated it
	tray       *mygo.Tray
	trayMenu   *mygo.Window
	trayClosed time.Time

	// control plane state, for the status bar
	mcpUp bool

	// the find bar over the terminal area
	findOpen bool
	findText string

	// the sidebar's session filter
	filter string

	// the audit window: the chosen result filter, the search text and the
	// selected call
	auditFilter string // "" or "all" | "ok" | "error" | "denied"
	auditQuery  string
	auditSel    string

	// fontFamilies caches the monospaced families the font setting offers.
	fontFamilies []myui.FontFamily

	// the settings window: a nav of categories, the content beside it
	settingsWin   *mygo.Window
	settingsCat   string   // 外观 | 终端 | 会话 | AI | MCP | 安全
	settingsNavW  float32  // the nav pane's width
	themeFilter   string   // the theme list's search box
	ghosttyThemes []string // the catalog, read once
	sizeText      string   // the font size input, until parsed
	setMCPPort    string   // the MCP port input, until parsed

	// the AI settings page's editing state: segment indices, text inputs and
	// the budget steppers live in fields because the widgets hold pointers
	// to them across frames
	aiProviderIdx int
	aiReasonIdx   int
	aiScopeIdx    int
	aiGuardIdx    int
	aiModelText   string
	aiBaseText    string
	aiKeyEnvText  string
	aiKeyText     string // the API key draft, before it is encrypted
	aiProbe       string // the connection probe's last outcome
	aiProbeOK     bool

	// the AI dock: its open state and dragged width, and the timeline the
	// agent will fill (empty until the loop lands in A5)
	aiDockOpen  bool
	aiDockW     float32
	aiDockSaved int // the width last written to config, to debounce drags
	aiInput     string
	aiOps       []aiOp

	// the AI operator's runtime: its agent, the turn's state and the token
	// total; mu guards the fields the agent goroutine writes
	aiMu         sync.Mutex
	aiAgent      *agent.Agent
	aiState      string // "" idle | thinking | tool | done | error
	aiErr        string
	aiTokenTotal int
	aiRunning    bool
	aiCancel     context.CancelFunc
	// aiClient lets a test replace the provider client; nil means the real one
	aiClient func(ai.Provider, string) *ai.Client

	// toolBackend is the shared tool layer: the MCP transport and the AI
	// agent both call it, so the guard, the mask and the audit cover both
	toolBackend *tools.Backend

	// the split panes of the active tab: a binary tree of leaves, one
	// session each, and the focused leaf, which activeID names too; zoom
	// is the leaf shown over the whole area, nil for none
	splitRoot *paneNode
	focus     *paneNode
	zoom      *paneNode

	// tabs are workspaces: each owns a split tree, so several terminals
	// live in one tab; activeTab is the one whose tree the fields above
	// mirror between frames
	tabs      []*workspace
	activeTab *workspace
	tabSeq    int

	// pending is the pane a split waits to fill with its new session,
	// even when the user switches to another tab meanwhile
	pending *panePending

	pendingToast string // shown by the next frame, from out-of-view errors
}

// paneNode is a leaf of the split tree, one session's terminal, or a
// split of two subtrees.
type paneNode struct {
	sessionID string // a leaf's session; "" while a split's session starts
	split     *splitNode
}

// workspace is one tab: its own split tree, the focused pane and the pane
// zoomed over it. A tab may hold several terminals.
type workspace struct {
	id       int
	root     *paneNode
	focus    *paneNode
	zoom     *paneNode
	activeID string
}

// panePending is a split's empty pane, waiting for the session it starts.
type panePending struct {
	tab  *workspace
	pane *paneNode
}

// splitNode is an interior node: two subtrees beside or above one
// another, the first taking a share of the room until the user drags the
// divider between them.
type splitNode struct {
	vertical bool // true: first|second side by side
	first    *paneNode
	second   *paneNode
	ratio    float32 // the first's share, 0.08–0.92
}

func newApp() *app {
	cfg, err := store.Load()
	if err != nil {
		cfg = &store.Config{Settings: store.Settings{MCPHTTP: true}}
		cfg.Settings.Normalize()
		log.Printf("config: %v", err)
	}
	// Older builds accepted a pasted API key in the "environment variable"
	// field; move it into the encrypted slot so it starts working.
	if migrateAIKeyEnv(cfg) {
		if err := cfg.Save(); err != nil {
			log.Printf("config: migrating AI key: %v", err)
		}
	}
	keys, err := sshc.LoadHostKeys(mustKnownHostsPath())
	if err != nil {
		log.Printf("known_hosts: %v", err)
		keys, _ = sshc.LoadHostKeys("")
	}
	auditLog, err := audit.Open(mustAuditLogPath())
	if err != nil {
		log.Printf("audit: %v", err)
	}
	aiCfg := cfg.Settings.AI
	ap := &app{
		mgr:           engine.NewManager(),
		shells:        shells.Detect(),
		sidebarWidth:  260,
		popouts:       map[string]*mygo.Window{},
		store:         cfg,
		hostKeys:      keys,
		auditLog:      auditLog,
		ghosttyThemes: terminal.GhosttyThemes(),
		settingsCat:   "外观",
		settingsNavW:  208,
		sizeText:      sizeTextFor(cfg.Settings.FontSize),
		setMCPPort:    portTextFor(cfg.Settings.MCPHTTPPort),
		aiProviderIdx: aiIndex(aiCfg.Provider, aiProviderOptions),
		aiReasonIdx:   aiIndex(aiCfg.Reasoning, aiReasoningOptions),
		aiScopeIdx:    aiIndex(aiCfg.ToolScope, aiToolScopeOptions),
		aiGuardIdx:    aiIndex(aiCfg.GuardMode, aiGuardModeOptions),
		aiModelText:   aiCfg.Model,
		aiBaseText:    aiCfg.BaseURL,
		aiKeyEnvText:  aiCfg.APIKeyEnv,
		aiDockOpen:    aiCfg.DockOpenOn(),
		aiDockW:       float32(aiCfg.DockWidth),
		aiDockSaved:   aiCfg.DockWidth,
		aiState:       "idle",
		guard: guard.FromSettings(cfg.Settings.GuardDisabled, cfg.Settings.GuardHeadless,
			cfg.Settings.GuardDeny, cfg.Settings.GuardConfirm),
	}
	// The guard asks a human in a dialog; with the AI operator and MCP
	// both calling a.guard, this is the one confirm channel.
	ap.guard.Confirm = ap.confirmGuard
	ap.guard.SetMode(guard.ModeFromString(aiCfg.GuardMode))
	// One tool backend for both entry points: MCP and the agent share the
	// guard, the agent mask and the audit.
	ap.toolBackend = tools.NewBackend(ap.mgr, cfg)
	ap.toolBackend.SetHostKeyPolicy(ap.sshPolicy())
	ap.toolBackend.SetAgentTracker(func(id, tool string) func() {
		s, ok := ap.mgr.Get(id)
		if !ok {
			return func() {}
		}
		s.SetAgentLock(true, tool)
		ap.refreshUI()
		return func() {
			s.SetAgentLock(false, "")
			ap.refreshUI()
		}
	})
	return ap
}

func sizeTextFor(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprint(n)
}

func portTextFor(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprint(n)
}

func mustAuditLogPath() string {
	p, err := store.AuditLogPath()
	if err != nil {
		return "audit.log"
	}
	return p
}

func mustKnownHostsPath() string {
	p, err := store.KnownHostsPath()
	if err != nil {
		return "known_hosts"
	}
	return p
}

func RunGUI() {
	// Every shell and program this app starts belongs to the app's job:
	// the system ends them when the app does.
	if err := procjob.Guard(); err != nil {
		log.Printf("process guard: %v", err)
	}
	registerBundledFonts()
	a := newApp()
	// The app's own text is Chinese; the framework's menus and dialogs
	// follow it until P23 adds an interface language setting.
	registerLocales()
	mygo.App.SetLocale("zh-CN")
	a.applyAppearance()
	a.mgr.Subscribe(a.onEngineEvent)
	mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
		a.quitting = true
		a.mgr.CloseAll()
	})
	mygo.App.WhenReady(func() {
		a.applyFont()
		a.applyTheme()
		a.startControlPlane()
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:         "AdaKaki 控制台",
			Width:         1240,
			Height:        800,
			MinWidth:      900,
			MinHeight:     560,
			TitleBarStyle: mygo.TitleBarHidden,
			Content:       myui.View(a.mainView),
		})
		iconWindow(a.win)
		if runtime.GOOS == "darwin" {
			if err := mygo.App.Dock.SetIcon(appIconPNG); err != nil {
				log.Printf("dock icon: %v", err)
			}
		}
		a.setupTray()
		a.win.OnClose(func(e *mygo.CloseEvent) {
			// Ask once while sessions run; the quit dialog's button
			// closes again with quitArmed, and the app quits.
			if a.mgr.Count() > 0 && !a.quitArmed {
				e.PreventDefault()
				a.win.Update(func() { a.showQuit = true })
			}
		})
		// Restore once the window exists: newSession refreshes it, and
		// the shell sessions open in the background.
		a.restoreSessions()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// onEngineEvent refreshes the windows for engine events. It runs on the
// engine's goroutines; Update does the work on the main thread.
func (a *app) onEngineEvent(ev engine.Event) {
	win := a.win
	if win == nil {
		return
	}
	win.Update(func() {
		switch ev.Kind {
		case engine.EventCreated:
			a.placeCreated(ev.ID)
			a.saveRestoreList()
		case engine.EventClosed:
			a.saveRestoreList()
			// A pane of a closed session has nothing to show: collapse.
			if leaf := a.leafByID(ev.ID); leaf != nil {
				a.closePane(leaf, false)
			}
			// The last pane closed with sessions still open: show one, so
			// the area is not left empty beside a full sidebar.
			if a.splitRoot == nil && len(a.mgr.List()) > 0 {
				a.activateFallback()
			} else if ev.ID == a.activeID && a.focus == nil {
				a.activateFallback()
			}
			if w := a.popouts[ev.ID]; w != nil {
				w.Close() // its OnClosed frees the mirror
			}
		case engine.EventTitle:
			if w := a.popouts[ev.ID]; w != nil && ev.Title != "" {
				w.SetTitle(ev.Title + " — 同屏")
			}
		}
		// every event redraws, for titles and state marks
	})
}

// activateFallback shows the newest remaining session, or none.
func (a *app) activateFallback() {
	sessions := a.mgr.List()
	if len(sessions) > 0 {
		a.viewSession(sessions[len(sessions)-1].ID())
	} else {
		a.activeID = ""
	}
}

// refreshUI redraws the main window from another goroutine.
func (a *app) refreshUI() {
	if a.win != nil {
		a.win.Update(func() {})
	}
}

// agentBusy counts the sessions an AI agent is operating right now.
func (a *app) agentBusy() int {
	n := 0
	for _, s := range a.mgr.List() {
		if on, _ := s.AgentLock(); on {
			n++
		}
	}
	return n
}

// quit closes the whole application from the quit dialog.
func (a *app) quit() {
	a.quitArmed = true
	mygo.App.Quit()
}

func (a *app) active() *engine.Session {
	s, _ := a.mgr.Get(a.activeID)
	return s
}
