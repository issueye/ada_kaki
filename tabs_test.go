package main

import "testing"

// TestTabsHoldPanes keeps several terminals in one tab: a split fills
// another pane of the same tab, and a session opened from elsewhere gets
// a tab of its own.
func TestTabsHoldPanes(t *testing.T) {
	a := newTestApp()
	defer a.mgr.CloseAll()

	a.viewSession("s1")
	if len(a.tabs) != 1 || a.tabs[0].root.sessionID != "s1" {
		t.Fatalf("after s1: %d tabs, root %+v", len(a.tabs), a.tabs[0].root)
	}

	a.splitFocused(true)
	a.focus.sessionID = "s2" // the pane the split started
	a.activeID = "s2"
	a.saveActive()
	a.pending = nil
	if len(a.tabs) != 1 {
		t.Fatalf("the split made %d tabs, want 1", len(a.tabs))
	}
	if n := leafCount(a.tabs[0].root); n != 2 {
		t.Errorf("the tab holds %d panes, want 2", n)
	}

	a.viewSession("s3")
	if len(a.tabs) != 2 {
		t.Fatalf("a new session made %d tabs, want 2", len(a.tabs))
	}
	if a.tabs[1].root.sessionID != "s3" {
		t.Errorf("new tab's root = %+v, want s3", a.tabs[1].root)
	}

	a.activateTab(a.tabs[0])
	if a.activeID != "s2" {
		t.Errorf("active session = %q, want s2", a.activeID)
	}
	a.closeTab(a.tabs[1])
	if len(a.tabs) != 1 {
		t.Errorf("closeTab left %d tabs, want 1", len(a.tabs))
	}
}
