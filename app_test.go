package main

import (
	"testing"

	"adakaki/internal/engine"
	"adakaki/internal/store"
)

func newTestApp() *app {
	return &app{mgr: engine.NewManager(), store: &store.Config{}}
}

func TestPaneTreeViewAndSplit(t *testing.T) {
	a := newTestApp()
	defer a.mgr.CloseAll()

	// The first session becomes the root leaf and the focus.
	a.viewSession("s1")
	if a.splitRoot == nil || a.splitRoot != a.focus || a.splitRoot.sessionID != "s1" {
		t.Fatalf("after viewSession: root=%+v focus=%+v", a.splitRoot, a.focus)
	}

	// A split turns the focused leaf into a node with a pending second.
	a.splitFocused(true)
	if a.splitRoot.split == nil || !a.splitRoot.split.vertical {
		t.Fatal("splitFocused did not split vertically")
	}
	if a.focus != a.splitRoot.split.second || a.focus.sessionID != "" {
		t.Fatalf("focus = %+v; want the pending second pane", a.focus)
	}
	if a.splitRoot.split.first.sessionID != "s1" {
		t.Errorf("first pane = %q; want s1", a.splitRoot.split.first.sessionID)
	}

	// The started session fills the pending pane.
	a.viewSession("s2")
	if a.focus.sessionID != "s2" || a.activeID != "s2" {
		t.Fatalf("after fill: focus=%+v active=%q", a.focus, a.activeID)
	}

	// A second split nests: first|second each split again.
	a.splitFocused(false)
	nodeX := a.splitRoot.split.second
	if nodeX.split == nil || nodeX.split.vertical {
		t.Fatal("the focused pane did not split horizontally")
	}
	if a.focus != nodeX.split.second || a.focus.sessionID != "" {
		t.Fatalf("focus = %+v; want the pending second pane", a.focus)
	}

	// Viewing an existing pane's session just moves the focus.
	if leaf := a.leafByID("s1"); leaf == nil {
		t.Fatal("s1 pane lost")
	}
	a.viewSession("s1") // s1 comes back into the pane
	if a.focus.sessionID != "s1" || a.activeID != "s1" {
		t.Fatalf("focus = %+v active = %q; want s1", a.focus, a.activeID)
	}
}

func TestPaneTreeCollapse(t *testing.T) {
	a := newTestApp()
	defer a.mgr.CloseAll()

	a.viewSession("s1") // s1 comes back into the pane
	a.splitFocused(true)  // root: s1 | pending
	a.viewSession("s2")   // pending fills with s2
	a.splitFocused(false) // s2's pane: s2 / pending
	a.viewSession("s3")

	// Tree: root[s1 | (s2 / s3)], focus on s3.
	if a.leafByID("s3") != a.focus {
		t.Fatal("s3 should be focused")
	}

	// Closing s3's pane collapses s2's split; focus moves to s2.
	a.closePane(a.focus, true)
	if a.splitRoot.split == nil {
		t.Fatal("root should still be a split of s1|s2")
	}
	if a.focus == nil || a.focus.sessionID != "s2" || a.activeID != "s2" {
		t.Fatalf("focus = %+v active = %q; want s2", a.focus, a.activeID)
	}

	// Closing s2's pane leaves the root a plain s1 leaf.
	s2 := a.leafByID("s2")
	a.closePane(s2, true)
	if a.splitRoot == nil || a.splitRoot.split != nil || a.splitRoot.sessionID != "s1" {
		t.Fatalf("root = %+v; want the s1 leaf", a.splitRoot)
	}
	if a.focus != a.splitRoot || a.activeID != "s1" {
		t.Fatalf("focus = %+v active = %q; want the s1 leaf", a.focus, a.activeID)
	}

	// Closing the last pane empties the area.
	a.closePane(a.splitRoot, true)
	if a.splitRoot != nil || a.focus != nil || a.activeID != "" {
		t.Fatalf("after last close: root=%+v focus=%+v active=%q", a.splitRoot, a.focus, a.activeID)
	}
}

func TestPaneTreeClosedSessionCollapses(t *testing.T) {
	a := newTestApp()
	defer a.mgr.CloseAll()

	a.viewSession("s1") // s1 comes back into the pane
	a.splitFocused(true)
	a.viewSession("s2")

	// The closed event of a pane's session collapses that pane, keep=true.
	a.closePane(a.leafByID("s2"), true)
	if a.splitRoot.split != nil || a.splitRoot.sessionID != "s1" {
		t.Fatalf("root = %+v; want the s1 leaf", a.splitRoot)
	}
	if a.focus != a.splitRoot || a.activeID != "s1" {
		t.Fatalf("focus = %+v active = %q", a.focus, a.activeID)
	}

	// leafByID finds nothing once the pane is gone.
	if a.leafByID("s2") != nil {
		t.Error("s2 pane still findable")
	}
}

func TestSplitSession(t *testing.T) {
	a := newTestApp()
	defer a.mgr.CloseAll()

	a.viewSession("s1")
	a.viewSession("s2") // s2 takes the focused leaf; s1 stays only in tabs
	a.viewSession("s1") // s1 comes back into the pane

	// Splitting by session ID acts on that session's pane, whatever the
	// focus was, and focuses the new pending pane.
	a.splitSession("s1", true)
	if a.splitRoot.split == nil || !a.splitRoot.split.vertical {
		t.Fatal("splitSession did not split the session's pane")
	}
	if a.focus != a.splitRoot.split.second || a.activeID != "" {
		t.Fatalf("focus = %+v; want the pending pane", a.focus)
	}
	a.viewSession("s2") // fill the pending pane

	// s1's pane is still the first leaf; splitting it again nests deeper.
	a.splitSession("s1", false)
	first := a.splitRoot.split.first
	if first.split == nil || first.split.vertical {
		t.Fatal("s1's pane did not split horizontally")
	}

	// A session that is not in any pane does nothing.
	a.splitSession("nope", true)
}
