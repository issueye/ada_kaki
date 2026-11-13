package main

import (
	"fmt"
	"log"
	"runtime"
	"time"

	"adakaki/internal/audit"
	"adakaki/internal/engine"
	"adakaki/internal/guard"
	"adakaki/internal/procjob"
	"adakaki/internal/shells"
	"adakaki/internal/sshc"
	"adakaki/internal/store"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
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

	// dialog state, for ui.Modal
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
	fontFamilies []ui.FontFamily

	// the settings window: a nav of categories, the content beside it
	settingsWin   *mygo.Window
	settingsCat   string   // 外观 | 终端 | 会话 | MCP | 安全
	settingsNavW  float32  // the nav pane's width
	themeFilter   string   // the theme list's search box
	ghosttyThemes []string // the catalog, read once
	sizeText      string   // the font size input, until parsed
	setMCPPort    string   // the MCP port input, until parsed

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

func newLeaf(sessionID string) *paneNode { return &paneNode{sessionID: sessionID} }

func newApp() *app {
	cfg, err := store.Load()
	if err != nil {
		cfg = &store.Config{Settings: store.Settings{MCPHTTP: true}}
		log.Printf("config: %v", err)
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
	return &app{
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
		guard: guard.FromSettings(cfg.Settings.GuardDisabled, cfg.Settings.GuardHeadless,
			cfg.Settings.GuardDeny, cfg.Settings.GuardConfirm),
	}
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

// confirmGuard runs on a tool-call goroutine: the dialog on the main
// thread, the answer back through a channel.
func (a *app) confirmGuard(req guard.Request, rule guard.Rule) bool {
	if a.win == nil {
		return false
	}
	ch := make(chan bool, 1)
	a.win.Update(func() {
		r, rl := req, rule
		a.guardReq, a.guardRule, a.guardCh = &r, &rl, ch
		a.showGuardAsk = true
	})
	return <-ch
}

func (a *app) answerGuard(ok bool) {
	if a.guardCh != nil {
		a.guardCh <- ok
		a.guardCh = nil
	}
	a.showGuardAsk = false
}

// openAudit shows the audit trail viewer in its own window.
func (a *app) openAudit() {
	if a.win == nil || a.auditWin != nil {
		return
	}
	a.auditWin = mygo.NewWindow(mygo.WindowOptions{
		Title:         "MCP 审计日志",
		TitleBarStyle: mygo.TitleBarHidden,
		Width:         1100,
		Height:        720,
		Content: ui.View(func(c *ui.Context) {
			a.auditView(c)
		}),
	})
	iconWindow(a.auditWin)
	a.auditWin.OnClosed(func() { a.auditWin = nil })
}

// ---- session restore ----

// saveRestoreList writes the sessions worth reopening into the config,
// so a restart brings them back. It runs on the main thread.
func (a *app) saveRestoreList() {
	specs := a.mgr.RestoreSpecs()
	out := make([]store.RestoreSpec, len(specs))
	for i, sp := range specs {
		out[i] = store.RestoreSpec{
			Kind: sp.Kind, Shell: sp.Shell, Cwd: sp.Cwd,
			Custom: sp.Custom, Profile: sp.Profile,
		}
	}
	a.store.Restore = out
	if err := a.store.Save(); err != nil {
		log.Printf("save restore list: %v", err)
	}
}

// restoreSessions reopens what the last run had, off the main thread per
// session; the last one takes the view, as it had at exit.
func (a *app) restoreSessions() {
	if a.store.Settings.RestoreDisabled {
		return
	}
	specs := a.store.Restore
	if len(specs) == 0 {
		return
	}
	for _, spec := range specs {
		switch spec.Kind {
		case "local":
			shell := shells.ID(spec.Shell)
			if shell == "" {
				continue
			}
			a.newSession(shell, spec.Cwd, spec.Custom)
		case "ssh":
			if spec.Profile == "" {
				continue // a direct-password session cannot re-auth
			}
			a.connectSSHProfile(spec.Profile)
		}
	}

}

// sshSourceFor builds the dial for an SSH profile: the stored secret
// (or the agent, or the key) and the host key policy.
func (a *app) sshSourceFor(p store.Profile) engine.SSHSource {
	auth := sshc.Auth{Kind: p.AuthKind, KeyPath: p.KeyPath}
	if auth.Kind == "" {
		auth.Kind = "password"
	}
	if p.Secret != "" {
		if plain, err := store.Unprotect(p.Secret); err == nil {
			if auth.Kind == "password" {
				auth.Password = plain
			} else if auth.Kind == "key" {
				auth.Passphrase = plain
			}
		}
	}
	policy := a.sshPolicy()
	return func(s *engine.Session) (engine.Conn, string, error) {
		client, err := sshc.Dial(p.Host, p.Port, p.User, auth, policy)
		if err != nil {
			return nil, "", err
		}
		conn, err := client.OpenShell(80, 24)
		if err != nil {
			_ = client.Close()
			return nil, "", err
		}
		return conn, p.User + "@" + p.Host, nil
	}
}

// connectSSHProfile opens a session from a saved profile, by name.
func (a *app) connectSSHProfile(name string) {
	p, ok := a.store.Profile(name)
	if !ok {
		return
	}
	go func() {
		source := a.sshSourceFor(p)
		if _, err := a.mgr.CreateSSH(p.Name, source); err != nil {
			a.win.Update(func() { a.pendingToast = err.Error() })
		}
	}()
}

// ---- keyboard shortcuts ----

// openFind shows the find bar and searches for what it holds.
func (a *app) openFind() {
	a.findOpen = true
	if a.findText != "" {
		a.searchFor(a.findText)
	}
}

func (a *app) closeFind() {
	a.findOpen = false
	if s := a.active(); s != nil {
		s.Search("") // end the search, clear the highlights
	}
}

func (a *app) searchFor(text string) {
	if s := a.active(); s != nil {
		s.Search(text)
	}
}

func (a *app) searchNext() {
	if s := a.active(); s != nil {
		s.SearchNext()
	}
}

func (a *app) searchPrev() {
	if s := a.active(); s != nil {
		s.SearchPrevious()
	}
}

// findStatus renders the find bar's counter.
func (a *app) findStatus() string {
	s := a.active()
	if s == nil {
		return ""
	}
	st := s.SearchStatus()
	switch {
	case st.Text == "":
		return ""
	case st.Searching:
		return "搜索中…"
	case st.Total == 0:
		return "无匹配"
	default:
		return fmt.Sprintf("%d / %d", st.Selected, st.Total)
	}
}

func mustKnownHostsPath() string {
	p, err := store.KnownHostsPath()
	if err != nil {
		return "known_hosts"
	}
	return p
}

// sshPolicy builds the host key policy from the settings: "prompt" asks
// in a dialog (the default), "accept_new" stores unknown keys, "off"
// verifies nothing. A CHANGED key is refused under every setting.
func (a *app) sshPolicy() *sshc.HostKeyPolicy {
	p := &sshc.HostKeyPolicy{Store: a.hostKeys}
	switch a.store.Settings.StrictHostKey {
	case "off":
		p.Insecure = true
	case "accept_new":
		p.AcceptNew = true
	default:
		p.Prompt = a.promptHostKey
	}
	return p
}

// promptHostKey runs on a dial goroutine: it raises the dialog on the
// main thread and blocks until the user answers.
func (a *app) promptHostKey(host string, port int, fp string) bool {
	ch := make(chan bool, 1)
	if a.win == nil {
		return false
	}
	a.win.Update(func() {
		a.hkHost, a.hkPort, a.hkFP, a.hkCh = host, port, fp, ch
		a.showHostKey = true
	})
	return <-ch
}

func (a *app) answerHostKey(ok bool) {
	if a.hkCh != nil {
		a.hkCh <- ok
		a.hkCh = nil
	}
	a.showHostKey = false
}

func runGUI() {
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
			Content:       ui.View(a.mainView),
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

// ---- split panes ----

// saveActive writes the active tab's fields into its workspace, so a
// switch or a frame that shows another tab keeps what the user set.
func (a *app) saveActive() {
	ws := a.activeTab
	if ws == nil {
		return
	}
	ws.root, ws.focus, ws.zoom, ws.activeID = a.splitRoot, a.focus, a.zoom, a.activeID
}

// loadActive makes ws the active tab and mirrors its fields.
func (a *app) loadActive(ws *workspace) {
	a.activeTab = ws
	if ws == nil {
		a.splitRoot, a.focus, a.zoom, a.activeID = nil, nil, nil, ""
		return
	}
	a.splitRoot, a.focus, a.zoom, a.activeID = ws.root, ws.focus, ws.zoom, ws.activeID
}

// openTab starts a new tab with id in its only pane, and activates it.
func (a *app) openTab(id string) *workspace {
	a.saveActive()
	a.tabSeq++
	ws := &workspace{id: a.tabSeq}
	leaf := newLeaf(id)
	ws.root, ws.focus, ws.activeID = leaf, leaf, id
	a.tabs = append(a.tabs, ws)
	a.loadActive(ws)
	return ws
}

// activateTab brings a tab forward, with the session its own focus had.
func (a *app) activateTab(ws *workspace) {
	if ws == nil || ws == a.activeTab {
		return
	}
	a.saveActive()
	a.loadActive(ws)
	if a.focus != nil {
		a.activeID = a.focus.sessionID
	}
}

// dropTab removes a workspace from the strip. When it was active, the
// nearest other tab takes its place, or the window is left with none.
func (a *app) dropTab(ws *workspace) {
	idx := -1
	for i, w := range a.tabs {
		if w == ws {
			idx = i
			break
		}
	}
	if idx >= 0 {
		a.tabs = append(a.tabs[:idx], a.tabs[idx+1:]...)
	}
	if idx < 0 {
		if a.activeTab == ws {
			a.loadActive(nil)
		}
		return
	}
	if a.activeTab != ws {
		return
	}
	if len(a.tabs) > 0 {
		next := a.tabs[min(idx, len(a.tabs)-1)]
		a.loadActive(next)
		if a.focus == nil {
			a.focus = a.firstLeaf(a.splitRoot)
		}
		if a.focus != nil {
			a.activeID = a.focus.sessionID
		}
	} else {
		a.loadActive(nil)
	}
}

// placeCreated puts a freshly created session where the UI meant it to
// go: the pending pane of a split, even if another tab was selected
// meanwhile, or a tab of its own.
func (a *app) placeCreated(id string) {
	if p := a.pending; p != nil {
		a.pending = nil
		if containsPane(p.tab.root, p.pane) && p.pane.sessionID == "" {
			if p.tab != a.activeTab {
				a.saveActive()
				a.loadActive(p.tab)
			}
			p.pane.sessionID = id
			a.focus = p.pane
			a.activeID = id
			a.zoom = nil
			a.saveActive()
			return
		}
	}
	a.viewSession(id)
}

// viewSession shows a session: the tab that already holds it comes
// forward, a pending pane of the active tab takes a new session, and
// anything else opens a tab of its own.
func (a *app) viewSession(id string) {
	for _, ws := range a.tabs {
		if leaf := leafByID(ws.root, id); leaf != nil {
			if ws != a.activeTab {
				a.saveActive()
				a.loadActive(ws)
			}
			if a.zoom != nil && a.zoom != leaf {
				a.zoom = nil // the split comes back with the pane chosen
			}
			a.focus = leaf
			a.activeID = id
			a.saveActive()
			return
		}
	}
	if a.splitRoot == nil {
		if a.activeTab == nil {
			a.openTab(id)
		} else {
			leaf := newLeaf(id)
			a.splitRoot, a.focus, a.activeID = leaf, leaf, id
		}
	} else if a.focus != nil && a.focus.split == nil && a.focus.sessionID == "" {
		a.focus.sessionID = id
		a.activeID = id
	} else {
		a.openTab(id)
	}
	a.saveActive()
}

// splitFocused turns the focused pane into a split, whose new pane takes
// a session of the default shell once it starts.
func (a *app) splitFocused(vertical bool) {
	if a.focus == nil || a.focus.split != nil {
		return
	}
	a.zoom = nil // a split is what the user wants to see
	old := *a.focus
	a.focus.split = &splitNode{
		vertical: vertical,
		first:    &old,
		second:   newLeaf(""),
		ratio:    0.5,
	}
	a.focus.sessionID = ""
	a.focus = a.focus.split.second
	a.activeID = ""
	a.pending = &panePending{tab: a.activeTab, pane: a.focus}
	a.saveActive()
	a.newSession(a.defaultShellID(), "", "")
}

// splitSession splits the pane a session lives in, focusing it first:
// the session menu's way to split, from wherever the right click was.
func (a *app) splitSession(id string, vertical bool) {
	leaf := a.leafByID(id)
	if leaf == nil || leaf.split != nil {
		return
	}
	if ws := a.tabOf(leaf); ws != nil && ws != a.activeTab {
		a.activateTab(ws)
	}
	a.focus = leaf
	a.activeID = id
	a.splitFocused(vertical)
}

// closePane closes a pane: its session with it, unless keep says the
// session is already gone.
func (a *app) closePane(n *paneNode, keep bool) {
	if n == nil || n.split != nil {
		return
	}
	if n.sessionID != "" && !keep {
		if s, ok := a.mgr.Get(n.sessionID); ok {
			s.Close() // the closed event collapses the pane again, keep=true
			return
		}
	}
	a.removePane(n)
}

// removePane collapses a pane out of its tab's tree and moves the focus
// to the pane that takes its place; a tab left with no pane closes.
func (a *app) removePane(n *paneNode) {
	ws := a.tabOf(n)
	if ws == nil {
		return
	}
	active := ws == a.activeTab
	if ws.zoom == n {
		ws.zoom = nil
		if active {
			a.zoom = nil
		}
	}
	if ws.root == n {
		ws.root, ws.focus, ws.activeID = nil, nil, ""
		if active {
			a.splitRoot, a.focus, a.zoom, a.activeID = nil, nil, nil, ""
		}
		a.dropTab(ws)
		return
	}
	parent := a.findParent(ws.root, n)
	if parent == nil || parent.split == nil {
		return
	}
	other := parent.split.first
	if other == n {
		other = parent.split.second
	}
	*parent = *other // the sibling takes the parent's place in the tree
	if ws.focus == n || ws.focus == nil {
		ws.focus = a.firstLeaf(parent)
	}
	if active {
		a.splitRoot = ws.root
		a.focus = ws.focus
		if a.focus != nil {
			a.activeID = a.focus.sessionID
		} else {
			ws.activeID = ""
			a.activeID = ""
		}
	}
	a.saveActive()
}

// leafByID finds the pane showing a session, in any tab, empty panes
// included.
func (a *app) leafByID(id string) *paneNode {
	for _, ws := range a.tabs {
		if l := leafByID(ws.root, id); l != nil {
			return l
		}
	}
	return nil
}

// tabOf finds the tab that owns a pane.
func (a *app) tabOf(n *paneNode) *workspace {
	for _, ws := range a.tabs {
		if containsPane(ws.root, n) {
			return ws
		}
	}
	return nil
}

// containsPane reports whether n is anywhere in the tree rooted at root.
func containsPane(root, n *paneNode) bool {
	if root == nil || n == nil {
		return false
	}
	if root == n {
		return true
	}
	s := root.split
	if s == nil {
		return false
	}
	return containsPane(s.first, n) || containsPane(s.second, n)
}

func leafByID(n *paneNode, id string) *paneNode {
	if n == nil {
		return nil
	}
	if n.split == nil {
		if n.sessionID == id {
			return n
		}
		return nil
	}
	if l := leafByID(n.split.first, id); l != nil {
		return l
	}
	return leafByID(n.split.second, id)
}

func (a *app) firstLeaf(n *paneNode) *paneNode {
	if n == nil {
		return nil
	}
	if n.split == nil {
		return n
	}
	if l := a.firstLeaf(n.split.first); l != nil {
		return l
	}
	return a.firstLeaf(n.split.second)
}

func (a *app) findParent(root, n *paneNode) *paneNode {
	if root == nil || root.split == nil {
		return nil
	}
	if root.split.first == n || root.split.second == n {
		return root
	}
	if p := a.findParent(root.split.first, n); p != nil {
		return p
	}
	return a.findParent(root.split.second, n)
}

// defaultShellID is the shell new panes start, the setting or the first
// shell found.
func (a *app) defaultShellID() shells.ID {
	if s := shells.ID(a.store.Settings.DefaultShell); s != "" {
		if sh, ok := shells.Get(s); ok && sh.Found {
			return s
		}
	}
	for _, sh := range a.shells {
		if sh.Found {
			return sh.ID
		}
	}
	return shells.Cmd
}

// newSession starts a local shell session off the main thread; the engine
// event activates it, and a failure lands in a pending toast.
func (a *app) newSession(id shells.ID, cwd, custom string) {
	win := a.win
	if win == nil {
		return
	}
	pending := a.pending
	go func() {
		_, err := a.mgr.CreateLocal(id, cwd, custom)
		if err != nil {
			win.Update(func() {
				if pending != nil && a.pending == pending {
					a.pending = nil // the split's pane stays empty
				}
				a.pendingToast = err.Error()
			})
		}
	}()
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

func (a *app) closeSession(s *engine.Session) {
	// The agent's session is its workspace: closing it would pull the
	// work out from under the agent.
	if on, _ := s.AgentLock(); on {
		a.pendingToast = "该会话正被 AI Agent 操作，已阻止关闭"
		return
	}
	if w := a.popouts[s.ID()]; w != nil {
		delete(a.popouts, s.ID())
		w.Close()
	}
	s.Close()
}

// forceCloseSession ends a local shell's whole process tree at once, for
// a program that ignores the hangup (a Java service, say) or takes too
// long to stop. The agent's lock still protects its workspace.
func (a *app) forceCloseSession(s *engine.Session) {
	if s.State() == engine.Exited {
		a.closeSession(s)
		return
	}
	if on, _ := s.AgentLock(); on {
		a.pendingToast = "该会话正被 AI Agent 操作，已阻止关闭"
		return
	}
	if w := a.popouts[s.ID()]; w != nil {
		delete(a.popouts, s.ID())
		w.Close()
	}
	if err := s.ForceClose(); err != nil {
		a.pendingToast = "强制关闭失败：" + err.Error()
		a.refreshUI()
	}
}

// popout opens a synchronized window for the session: a second terminal
// fed by the session's Mirror frames, whose keys go to the session. The
// main window's tab keeps the session's own view, so both ends show the
// same program and take input at once.
func (a *app) popout(s *engine.Session) {
	if a.win == nil || a.popouts[s.ID()] != nil {
		return
	}
	mv, err := s.OpenMirror()
	if err != nil {
		a.pendingToast = err.Error()
		return
	}
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:         s.Title() + " — 同屏",
		TitleBarStyle: mygo.TitleBarHidden,
		Width:         940,
		Height:        600,
		Content: ui.View(func(c *ui.Context) {
			a.popoutView(c, mv, s.Title())
		}),
	})
	iconWindow(w)
	a.popouts[s.ID()] = w
	w.OnClosed(func() {
		mv.Close()
		delete(a.popouts, s.ID())
		if a.win != nil {
			a.win.Invalidate()
		}
	})
}

// recall closes the popped-out window of a session, back to its tab.
func (a *app) recall(s *engine.Session) {
	if w := a.popouts[s.ID()]; w != nil {
		w.Close()
	}
}

// reconnect restarts an exited session in place: an SSH session dials
// again, a local one starts its shell or command again; the pane, the
// tab and the sidebar entry all stay where they are.
func (a *app) reconnect(s *engine.Session) {
	if on, _ := s.AgentLock(); on {
		a.pendingToast = "该会话正被 AI Agent 操作，已阻止重连"
		return
	}
	go func() {
		err := s.Reconnect()
		if err == nil {
			// The old mirror froze at the last screen: a fresh window
			// after the reconnect shows the fresh session.
			a.win.Update(func() {
				if w := a.popouts[s.ID()]; w != nil {
					delete(a.popouts, s.ID())
					w.Close()
				}
			})
			return
		}
		a.win.Update(func() { a.pendingToast = "重连失败：" + err.Error() })
	}()
}

// connectSSH starts an SSH session off the main thread, from the dialog's
// fields; the profile name goes to the session, and Save archives it.
func (a *app) connectSSH() {
	authKind := a.sshAuth
	if authKind == "" {
		authKind = "password"
	}
	p := store.Profile{
		Name:     a.sshProfile,
		Host:     a.sshHost,
		User:     a.sshUser,
		AuthKind: authKind,
		KeyPath:  a.sshKeyPath,
	}
	fmt.Sscanf(a.sshPort, "%d", &p.Port)
	if p.Host == "" || p.User == "" {
		a.pendingToast = "SSH：主机和用户名不能为空"
		return
	}
	if a.sshSave && a.sshSaveAs != "" {
		p.Name = a.sshSaveAs
		if authKind != "agent" {
			if c, err := store.Protect(a.sshPass); err == nil {
				p.Secret = c // the password, or the key's passphrase
			}
		}
		if err := a.store.PutProfile(p); err != nil {
			log.Printf("save profile: %v", err)
		}
	}

	auth := sshc.Auth{Kind: p.AuthKind, KeyPath: p.KeyPath, Passphrase: a.sshPass}
	if authKind == "password" {
		auth.Password = a.sshPass
		// A saved password stays hidden: an empty field uses the archive.
		if a.sshPass == "" && a.sshProfile != "" {
			if saved, ok := a.store.Profile(a.sshProfile); ok && saved.AuthKind == "password" {
				if plain, err := store.Unprotect(saved.Secret); err == nil {
					auth.Password = plain
				}
			}
		}
	}
	name := p.Name
	a.showSSH = false

	go func() {
		source := func(s *engine.Session) (engine.Conn, string, error) {
			client, err := sshc.Dial(p.Host, p.Port, p.User, auth, a.sshPolicy())
			if err != nil {
				return nil, "", err
			}
			conn, err := client.OpenShell(80, 24)
			if err != nil {
				_ = client.Close()
				return nil, "", err
			}
			return conn, p.User + "@" + p.Host, nil
		}
		if _, err := a.mgr.CreateSSH(name, source); err != nil {
			a.win.Update(func() { a.pendingToast = err.Error() })
		}
	}()
}

// quit closes the whole application from the quit dialog.
func (a *app) quit() {
	a.quitArmed = true
	mygo.App.Quit()
}

// ---- settings page ----

// openSettings opens the settings window: a nav of categories on the
// left, the category's content on the right. Changes apply live and save
// as they are made.
func (a *app) openSettings() {
	if a.win == nil || a.settingsWin != nil {
		return
	}
	a.settingsCat = "外观"
	a.settingsWin = mygo.NewWindow(mygo.WindowOptions{
		Title:         "设置",
		TitleBarStyle: mygo.TitleBarHidden,
		Width:         1040,
		Height:        720,
		Content: ui.View(func(c *ui.Context) {
			a.settingsView(c)
		}),
	})
	iconWindow(a.settingsWin)
	a.settingsWin.OnClosed(func() { a.settingsWin = nil })
}

// saveSettingsNow persists the configuration; the settings page calls it
// after every change.
func (a *app) saveSettingsNow() {
	if err := a.store.Save(); err != nil {
		a.pendingToast = "保存设置失败：" + err.Error()
		a.refreshUI()
	}
}

// applyFont pushes the configured font to the engine: new sessions start
// with it, running ones switch live.
func (a *app) applyFont() {
	a.mgr.SetFont(terminal.Font{
		Family: a.store.Settings.FontFamily,
		Size:   float32(a.store.Settings.FontSize),
	})
}

// applyTheme pushes the configured color scheme to the engine; "" means
// follow the window's appearance.
func (a *app) applyTheme() {
	if err := a.mgr.SetTheme(a.store.Settings.TerminalTheme); err != nil {
		a.pendingToast = "配色方案 " + a.store.Settings.TerminalTheme + " 不存在"
		a.refreshUI()
	}
}

// appearanceSource maps the setting to the framework's theme source.
func (a *app) appearanceSource() mygo.ThemeSource {
	switch a.store.Settings.Appearance {
	case "dark":
		return mygo.ThemeDark
	case "light":
		return mygo.ThemeLight
	}
	return mygo.ThemeSystem
}

// applyAppearance tells the framework the app's appearance: the palette,
// the window controls of hidden title bars, the menus and the terminals'
// light or dark defaults all follow it.
func (a *app) applyAppearance() {
	mygo.Theme.SetSource(a.appearanceSource())
}
