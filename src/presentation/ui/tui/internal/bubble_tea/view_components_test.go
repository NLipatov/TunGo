package bubble_tea

import (
	"strings"
	"testing"
)

func TestRenderLogsBody_EmptyAndNonEmpty(t *testing.T) {
	empty := renderLogsBody(nil, 40)
	if len(empty) != 1 {
		t.Fatalf("expected one fallback line, got %v", empty)
	}

	lines := renderLogsBody([]string{"first", "second"}, 8)
	if len(lines) != 2 {
		t.Fatalf("expected two rendered lines, got %v", lines)
	}
	if lines[0] == "" || lines[1] == "" {
		t.Fatalf("expected non-empty rendered lines, got %v", lines)
	}
}

func TestTruncateWithEllipsis_EdgeCases(t *testing.T) {
	if got := truncateWithEllipsis("abcdef", 0); got != "abcdef" {
		t.Fatalf("expected unchanged for width<=0, got %q", got)
	}
	if got := truncateWithEllipsis("abcdef", 3); got != "abc" {
		t.Fatalf("expected hard truncate for very small width, got %q", got)
	}
	if got := truncateWithEllipsis("abcdef", 5); got != "ab..." {
		t.Fatalf("expected ellipsis truncate, got %q", got)
	}
}

func TestLogTailLimit_Adaptive(t *testing.T) {
	if got := logTailLimit(0); got != 8 {
		t.Fatalf("expected default limit 8, got %d", got)
	}
	if got := logTailLimit(200); got != 14 {
		t.Fatalf("expected upper clamp 14, got %d", got)
	}
	if got := logTailLimit(6); got != 4 {
		t.Fatalf("expected lower clamp 4 for tiny height, got %d", got)
	}
}

func TestUISettingsRows_UsesReadableStatsUnitsLabels(t *testing.T) {
	rows := uiSettingsRows(UIPreferences{
		Theme:              ThemeLight,
		Language:           "en",
		StatsUnits:         StatsUnitsBytes,
		ShowDataplaneStats: true,
		ShowDataplaneGraph: true,
		ShowFooter:         true,
	})
	if len(rows) != 5 {
		t.Fatalf("expected 5 settings rows, got %d", len(rows))
	}
	if !strings.Contains(rows[1], "Decimal units (KB/MB/GB)") {
		t.Fatalf("expected bytes label, got %q", rows[1])
	}

	rows = uiSettingsRows(UIPreferences{
		Theme:              ThemeLight,
		Language:           "en",
		StatsUnits:         StatsUnitsBiBytes,
		ShowDataplaneStats: true,
		ShowDataplaneGraph: true,
		ShowFooter:         true,
	})
	if !strings.Contains(rows[1], "Binary units (KiB/MiB/GiB)") {
		t.Fatalf("expected binary label, got %q", rows[1])
	}
}

func TestRenderTabsLine_RightAlignsProductLabelWhenWidthAllows(t *testing.T) {
	styles := resolveUIStyles(UIPreferences{Theme: ThemeDark})
	line := renderTabsLine(
		"TunGo [v0.9.0]",
		"selector",
		[]string{"Main", "Settings", "Logs"},
		0,
		60,
		styles,
	)

	plain := stripANSI(line)
	labelIndex := strings.Index(plain, "TunGo [v0.9.0]")
	mainIndex := strings.Index(plain, "Main")
	if labelIndex < 0 || mainIndex < 0 {
		t.Fatalf("expected both tabs and product label in header, got %q", plain)
	}
	if labelIndex <= mainIndex {
		t.Fatalf("expected product label on the right, got %q", plain)
	}
}

func TestRenderTabsLine_KeepProductLabelOnVeryNarrowWidth(t *testing.T) {
	styles := resolveUIStyles(UIPreferences{Theme: ThemeDark})
	line := renderTabsLine(
		"TunGo [v0.9.0]",
		"selector",
		[]string{"Main", "Settings", "Logs"},
		0,
		16,
		styles,
	)

	plain := stripANSI(line)
	if !strings.Contains(plain, "TunGo [v0.9.0]") {
		t.Fatalf("expected product label to remain visible on narrow width, got %q", plain)
	}
}

func TestTruncateWithEllipsis_MultiByte(t *testing.T) {
	s := "АБВГДЕЖЗИК" // 10 Cyrillic runes
	got := truncateWithEllipsis(s, 6)
	runes := []rune(got)
	if len(runes) != 6 {
		t.Fatalf("expected 6 runes, got %d: %q", len(runes), got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected ellipsis suffix for multi-byte truncation, got %q", got)
	}
}

func TestTruncateWithEllipsis_SmallWidths(t *testing.T) {
	s := "abcdef"
	if got := truncateWithEllipsis(s, 1); got != "a" {
		t.Fatalf("expected 'a' for width=1, got %q", got)
	}
	if got := truncateWithEllipsis(s, 2); got != "ab" {
		t.Fatalf("expected 'ab' for width=2, got %q", got)
	}
	if got := truncateWithEllipsis(s, 3); got != "abc" {
		t.Fatalf("expected 'abc' for width=3 (no ellipsis), got %q", got)
	}
}

func TestTruncateWithEllipsis_Width0ReturnsOriginal(t *testing.T) {
	s := "hello"
	if got := truncateWithEllipsis(s, 0); got != s {
		t.Fatalf("expected original for width=0, got %q", got)
	}
}

func TestTruncateWithEllipsis_ANSIContainingString(t *testing.T) {
	// ANSI strings use rune-based truncation path
	s := "\x1b[31mhello world\x1b[0m"
	got := truncateWithEllipsis(s, 8)
	runes := []rune(got)
	if len(runes) != 8 {
		t.Fatalf("expected 8 runes for ANSI string truncation, got %d: %q", len(runes), got)
	}
}

func TestIsASCIIString(t *testing.T) {
	if !isASCIIString("") {
		t.Fatal("expected empty string to be ASCII")
	}
	if !isASCIIString("hello world 123") {
		t.Fatal("expected pure ASCII string to return true")
	}
	if isASCIIString("hello \x80 world") {
		t.Fatal("expected non-ASCII rune to return false")
	}
	if isASCIIString("Привет") {
		t.Fatal("expected Cyrillic string to return false")
	}
}

func TestRuntimeLogSnapshot_ReusableInsufficientCapacity(t *testing.T) {
	feed := NewRuntimeLogBuffer(8)
	_, _ = feed.Write([]byte("one\ntwo\n"))

	// Reusable with small capacity forces reallocation.
	small := make([]string, 1)
	reusable := &small
	lines := runtimeLogSnapshot(feed, reusable)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(lines), lines)
	}
	if cap(*reusable) < runtimeLogViewportSnapshotLimit {
		t.Fatalf("expected reusable capacity to grow to %d, got %d", runtimeLogViewportSnapshotLimit, cap(*reusable))
	}
}

func TestRuntimeLogSnapshot_NilReusableAndNilFeed(t *testing.T) {
	// nil feed returns nil
	if got := runtimeLogSnapshot(nil, nil); got != nil {
		t.Fatalf("expected nil for nil feed, got %v", got)
	}

	// nil reusable with valid feed uses Tail directly
	feed := NewRuntimeLogBuffer(8)
	_, _ = feed.Write([]byte("one\n"))
	lines := runtimeLogSnapshot(feed, nil)
	if len(lines) != 1 || lines[0] != "one" {
		t.Fatalf("expected [one], got %v", lines)
	}
}

func TestComputeLogsViewportSize_NonPositiveHeight(t *testing.T) {
	_, h := computeLogsViewportSize(80, 0, CurrentUIPreferences(), "", "hint")
	if h != 8 {
		t.Fatalf("expected default height 8 for height<=0, got %d", h)
	}
	_, h = computeLogsViewportSize(80, -1, CurrentUIPreferences(), "", "hint")
	if h != 8 {
		t.Fatalf("expected default height 8 for negative height, got %d", h)
	}
}
