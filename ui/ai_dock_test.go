package ui

import (
	"testing"

	"adakaki/internal/engine"
	"adakaki/internal/store"

	myui "github.com/egoist/mygo/ui"
)

// newAITestApp is the smallest app the AI views need: a normalized
// configuration and a manager without sessions.
func newAITestApp(t *testing.T) *app {
	t.Helper()
	cfg := &store.Config{}
	cfg.Settings.Normalize()
	mgr := engine.NewManager()
	t.Cleanup(mgr.CloseAll)
	return &app{store: cfg, mgr: mgr, aiDockSaved: cfg.Settings.AI.DockWidth}
}

func TestAIDockClamp(t *testing.T) {
	cases := map[float32]float32{
		100:                  float32(store.AIDockWidthMin),
		-5:                   float32(store.AIDockWidthMin),
		300:                  300,
		340:                  340,
		store.AIDockWidthMax: float32(store.AIDockWidthMax),
		600:                  float32(store.AIDockWidthMax),
	}
	for in, want := range cases {
		if got := clampAIDockWidth(in); got != want {
			t.Fatalf("clampAIDockWidth(%v) = %v, want %v", in, got, want)
		}
	}
}

// The toggle updates state and, when persisting, the config file.
func TestSetAIDockPersists(t *testing.T) {
	a := newAITestApp(t)
	a.setAIDock(false, false)
	if a.aiDockOpen {
		t.Fatal("setAIDock(false, false) left the dock open")
	}

	t.Setenv("AppData", t.TempDir())
	a.setAIDock(true, true)
	if !a.aiDockOpen {
		t.Fatal("setAIDock(true, true) did not open the dock")
	}
	cfg, err := store.Load()
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if !cfg.Settings.AI.DockOpenOn() {
		t.Fatal("dock_open was not persisted")
	}
}

// The dock renders its head, the target line and the empty timeline, which
// is what a user sees before the agent has done anything.
func TestAIDockRendersEmptyState(t *testing.T) {
	a := newAITestApp(t)
	a.store.Settings.AI.Enabled = true
	a.aiDockOpen = true

	tt := myui.NewTester(func(c *myui.Context) { a.aiDock(c) }, 360, 720)
	tt.Frame()

	for _, want := range []string{"AI 操作", "还没有操作", "选择一个会话后开始", "发送"} {
		if !tt.HasText(want) {
			t.Fatalf("dock does not show %q; texts = %v", want, tt.Texts())
		}
	}
}

// The title-bar button only exists while the AI operator is enabled, and
// it carries the step badge once there are operations.
func TestAIButtonStates(t *testing.T) {
	a := newAITestApp(t)
	a.store.Settings.AI.Enabled = false
	tt := myui.NewTester(func(c *myui.Context) { a.aiButton(c) }, 400, 60)
	tt.Frame()
	if tt.HasText("AI 操作") {
		t.Fatal("the AI button must stay hidden while the operator is disabled")
	}

	a.store.Settings.AI.Enabled = true
	a.aiDockOpen = true
	a.aiOps = make([]aiOp, 3)
	tt = myui.NewTester(func(c *myui.Context) { a.aiButton(c) }, 400, 60)
	tt.Frame()
	if !tt.HasText("AI 操作") {
		t.Fatal("the AI button must be visible while the operator is enabled")
	}
	if !tt.HasText("3") {
		t.Fatalf("the badge should show the step count; texts = %v", tt.Texts())
	}
}

// The dock is the split's first pane laid out right to left, so it must
// occupy the right side of the content area.
func TestAIDockSitsOnTheRight(t *testing.T) {
	a := newAITestApp(t)
	a.store.Settings.AI.Enabled = true
	a.aiDockOpen = true
	a.aiDockW = 340

	tt := myui.NewTester(func(c *myui.Context) { a.contentPane(c) }, 1400, 800)
	tt.Frame()

	rect, ok := tt.Find("AI 操作")
	if !ok {
		t.Fatalf("dock head not found; texts = %v", tt.Texts())
	}
	if rect.X < 700 {
		t.Fatalf("dock head at x=%v, want the right half of a 1400 DIP window", rect.X)
	}
	if rect.W >= 400 {
		t.Fatalf("dock head width %v suggests the dock takes the whole area", rect.W)
	}
}

// With the operator disabled the content area is exactly the pre-AI
// layout: no dock, no split.
func TestAIDockHiddenWhenDisabled(t *testing.T) {
	a := newAITestApp(t)
	a.store.Settings.AI.Enabled = false
	a.aiDockOpen = true

	tt := myui.NewTester(func(c *myui.Context) { a.contentPane(c) }, 1400, 800)
	tt.Frame()
	if tt.HasText("AI 操作") {
		t.Fatalf("a disabled operator must not render the dock; texts = %v", tt.Texts())
	}
}
