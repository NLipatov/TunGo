package bubble_tea

import (
	"strings"
	"testing"
)

func TestRenderLogsBody_EmptyAndNonEmpty(t *testing.T) {
	styles := resolveUIStyles(newDefaultUIPreferencesProvider().Preferences())
	empty := renderLogsBody(nil, 40, styles)
	if len(empty) != 1 {
		t.Fatalf("expected one fallback line, got %v", empty)
	}

	lines := renderLogsBody([]string{"first", "second"}, 8, styles)
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
	if len(rows) != 6 {
		t.Fatalf("expected 6 settings rows (mode=not set, no auto-connect row), got %d", len(rows))
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
		ThemeDark,
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
		ThemeDark,
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
	s := newDefaultUIPreferencesProvider()
	prefs := s.Preferences()
	_, h := computeLogsViewportSize(80, 0, prefs, "", "hint")
	if h != 8 {
		t.Fatalf("expected default height 8 for height<=0, got %d", h)
	}
	_, h = computeLogsViewportSize(80, -1, prefs, "", "hint")
	if h != 8 {
		t.Fatalf("expected default height 8 for negative height, got %d", h)
	}
}

func TestComputeLogsViewportSize_PositiveHeight_WithSubtitle(t *testing.T) {
	s := newDefaultUIPreferencesProvider()
	prefs := s.Preferences()
	prefs.ShowFooter = true
	w, h := computeLogsViewportSize(100, 40, prefs, "Subtitle text", "hint")
	if w <= 0 {
		t.Fatalf("expected positive content width, got %d", w)
	}
	if h < 3 {
		t.Fatalf("expected viewport height >= 3, got %d", h)
	}
}

func TestRuntimeLogSnapshot_EmptyFeedWithReusable(t *testing.T) {
	feed := NewRuntimeLogBuffer(8)
	// Feed has no lines written.
	buf := make([]string, 10)
	reusable := &buf
	got := runtimeLogSnapshot(feed, reusable)
	if got != nil {
		t.Fatalf("expected nil for empty feed with reusable, got %v", got)
	}
}

func TestTruncateWithEllipsis_NonASCIIWithinWidth(t *testing.T) {
	s := "Привет" // 6 Cyrillic runes, fits in width 10
	got := truncateWithEllipsis(s, 10)
	if got != s {
		t.Fatalf("expected unchanged non-ASCII string within width, got %q", got)
	}
}

func TestTruncateWithEllipsis_NonASCII_SmallWidth(t *testing.T) {
	s := "АБВГДЕЖЗИК" // 10 runes, needs truncation
	// width=1 → just first rune, no ellipsis
	if got := truncateWithEllipsis(s, 1); got != "А" {
		t.Fatalf("expected single rune for width=1, got %q", got)
	}
	// width=3 → exactly 3 runes, no room for ellipsis
	if got := truncateWithEllipsis(s, 3); got != "АБВ" {
		t.Fatalf("expected 3 runes for width=3, got %q", got)
	}
}

func TestComputeLogsViewportSize_TinyHeight_ClampsTo3(t *testing.T) {
	s := newDefaultUIPreferencesProvider()
	prefs := s.Preferences()
	prefs.ShowFooter = true
	_, h := computeLogsViewportSize(100, 10, prefs, "Long subtitle text for testing", "hint")
	if h < 3 {
		t.Fatalf("expected viewport height >= 3, got %d", h)
	}
}
