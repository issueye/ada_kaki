// Tabs and split panes: the model that places sessions in panes,
// turns panes into splits and collapses them again.

package ui

func newLeaf(sessionID string) *paneNode { return &paneNode{sessionID: sessionID} }

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
