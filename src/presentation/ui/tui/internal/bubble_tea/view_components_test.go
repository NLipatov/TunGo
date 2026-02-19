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
		Theme:      ThemeLight,
		Language:   "en",
		StatsUnits: StatsUnitsBytes,
		ShowFooter: true,
	})
	if len(rows) != 3 {
		t.Fatalf("expected 3 settings rows, got %d", len(rows))
	}
	if !strings.Contains(rows[1], "Decimal units (KB/MB/GB)") {
		t.Fatalf("expected bytes label, got %q", rows[1])
	}

	rows = uiSettingsRows(UIPreferences{
		Theme:      ThemeLight,
		Language:   "en",
		StatsUnits: StatsUnitsBiBytes,
		ShowFooter: true,
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
