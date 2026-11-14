package ui

import "testing"

// aiIndex maps a stored value to a segment index and defaults to 0.
func TestAIIndex(t *testing.T) {
	cases := []struct {
		val  string
		opts []aiOption
		want int
	}{
		{"local", aiProviderOptions, 0},
		{"cloud", aiProviderOptions, 1},
		{"high", aiReasoningOptions, 3},
		{"readonly", aiToolScopeOptions, 2},
		{"allow", aiGuardModeOptions, 2},
		{"unknown", aiProviderOptions, 0},
		{"", aiReasoningOptions, 0},
	}
	for _, tc := range cases {
		if got := aiIndex(tc.val, tc.opts); got != tc.want {
			t.Fatalf("aiIndex(%q) = %d, want %d", tc.val, got, tc.want)
		}
	}
}

// aiValue maps back, and an out-of-range index falls back to the first
// choice instead of panicking.
func TestAIValue(t *testing.T) {
	if got := aiValue(1, aiProviderOptions); got != "cloud" {
		t.Fatalf("aiValue(1) = %q, want cloud", got)
	}
	if got := aiValue(-3, aiProviderOptions); got != "local" {
		t.Fatalf("aiValue(-3) = %q, want local", got)
	}
	if got := aiValue(99, aiGuardModeOptions); got != "confirm_all" {
		t.Fatalf("aiValue(99) = %q, want confirm_all", got)
	}
}

func TestAILabel(t *testing.T) {
	if got := aiLabel("high", aiReasoningOptions); got != "高" {
		t.Fatalf("aiLabel(high) = %q, want 高", got)
	}
	if got := aiLabel("mystery", aiProviderOptions); got != "mystery" {
		t.Fatalf("an unknown value should render as itself, got %q", got)
	}
}

func TestThousands(t *testing.T) {
	cases := map[int]string{
		0:         "0",
		999:       "999",
		1000:      "1,000",
		20000:     "20,000",
		200000:    "200,000",
		1000000:   "1,000,000",
		-12000:    "-12,000",
	}
	for n, want := range cases {
		if got := thousands(n); got != want {
			t.Fatalf("thousands(%d) = %q, want %q", n, got, want)
		}
	}
}
