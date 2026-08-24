package bubble_tea

import (
	"strings"
	"testing"
	"time"
)

func TestRenderLogsBody_EmptyAndNonEmpty(t *testing.T) {
	styles := resolveUIStyles(newDefaultPreferences().Current())
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

func TestRenderLogsViewportContent_GroupsAndWrapsStructuredLogs(t *testing.T) {
	styles := resolveUIStyles(newDefaultPreferences().Current())
	lines := []string{
		`time=2026-08-21T23:59:59.123+04:00 level=WARN msg="transient write error, packet dropped" err=timeout`,
		`time=2026-08-22T00:00:01.456+04:00 level=INFO msg=reconnected`,
	}

	rendered := strings.Join(renderLogsViewportLines(lines, 50, styles), "\n")
	if got := strings.Count(rendered, ansiFgBrightYellow+"│"+ansiReset); got != 2 {
		t.Fatalf("expected both WARN markers to be colored, got %d in %q", got, rendered)
	}

	got := strings.Split(stripANSI(rendered), "\n")
	if len(got) != 6 {
		t.Fatalf("expected two date dividers, three log rows, and a separator, got %q", got)
	}
	if !strings.Contains(got[0], "2026-08-21") || !strings.Contains(got[4], "2026-08-22") {
		t.Fatalf("expected a divider for each date, got %q", got)
	}
	if got[1] != "23:59:59.123 WARN  │ transient write error, packet" {
		t.Fatalf("unexpected first log row: %q", got[1])
	}
	if got[2] != "                   │ dropped err=timeout" {
		t.Fatalf("unexpected continuation row: %q", got[2])
	}
	if got[3] != "" {
		t.Fatalf("expected an empty line between log entries, got %q", got[3])
	}
	if got[5] != "00:00:01.456 INFO  │ reconnected" {
		t.Fatalf("unexpected second log row: %q", got[5])
	}
	if strings.Contains(strings.Join(got, "\n"), "time=2026-") {
		t.Fatalf("expected verbose timestamps to be removed, got %q", got)
	}
}

func TestRenderLogsViewportContent_ColorsMarkersByLevel(t *testing.T) {
	styles := resolveUIStyles(newDefaultPreferences().Current())
	lines := []string{
		`time=2026-08-21T10:00:00.000+04:00 level=INFO msg=connected`,
		`time=2026-08-21T10:00:01.000+04:00 level=WARN msg=retrying`,
		`time=2026-08-21T10:00:02.000+04:00 level=ERROR msg=failed`,
	}

	got := strings.Join(renderLogsViewportLines(lines, 80, styles), "\n")
	for level, marker := range map[string]string{
		"INFO":  ansiFgBrightGreen + "│" + ansiReset,
		"WARN":  ansiFgBrightYellow + "│" + ansiReset,
		"ERROR": ansiFgBrightRed + "│" + ansiReset,
	} {
		if !strings.Contains(got, marker) {
			t.Errorf("expected %s marker color in %q", level, got)
		}
	}
}

func TestParseSlogTextLine_DecodesQuotedMessage(t *testing.T) {
	line := `time=2026-08-21T10:00:00.000+04:00 level=ERROR msg="write \"failed\"\tpath=C:\\TunGo\nretry" err=timeout`

	_, _, message, ok := parseSlogTextLine(line)
	if !ok {
		t.Fatal("expected structured slog line to be parsed")
	}
	if want := "write \"failed\"    path=C:\\TunGo\nretry err=timeout"; message != want {
		t.Fatalf("message = %q, want %q", message, want)
	}
}

func TestParseSlogTextLine_SanitizesTerminalControls(t *testing.T) {
	line := `time=2026-08-21T10:00:00.000+04:00 level=ERROR msg="before\x1b[2J\r\b\aafter"`

	_, _, message, ok := parseSlogTextLine(line)
	if !ok {
		t.Fatal("expected structured slog line to be parsed")
	}
	if want := `before\x1b[2J\r\b\aafter`; message != want {
		t.Fatalf("message = %q, want %q", message, want)
	}
}

func TestRenderLogEntry_NarrowViewportKeepsColoredMarkers(t *testing.T) {
	timestamp, err := time.Parse(time.RFC3339Nano, "2026-08-21T10:00:00.000+04:00")
	if err != nil {
		t.Fatal(err)
	}

	rows := appendLogEntry(nil, timestamp, "WARN", "first message continues", 20)
	marker := ansiFgBrightYellow + "│" + ansiReset
	if len(rows) < 3 {
		t.Fatalf("expected metadata and wrapped message rows, got %q", rows)
	}
	for _, row := range rows[1:] {
		if !strings.HasPrefix(row, marker) {
			t.Errorf("expected colored marker on message row %q", row)
		}
		if width := visibleWidthANSI(row); width > 20 {
			t.Errorf("message row width = %d, want <= 20: %q", width, row)
		}
	}

	for _, width := range []int{1, 2} {
		rows := appendLogEntry(nil, timestamp, "WARN", "message", width)
		hasMarker := false
		for _, row := range rows {
			hasMarker = hasMarker || strings.Contains(row, marker)
			if rowWidth := visibleWidthANSI(row); rowWidth > width {
				t.Errorf("width %d: row width = %d: %q", width, rowWidth, row)
			}
		}
		if !hasMarker {
			t.Errorf("width %d: expected a colored marker in %q", width, rows)
		}
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
	}, true)
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
	}, true)
	if !strings.Contains(rows[1], "Binary units (KiB/MiB/GiB)") {
		t.Fatalf("expected binary label, got %q", rows[1])
	}
}

func TestRenderTabsLine_RightAlignsProductLabelWhenWidthAllows(t *testing.T) {
	styles := resolveUIStyles(UIPreferences{Theme: ThemeDark})
	line := renderTabsLine(
		"TunGo [v0.9.0]",
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
	s := newDefaultPreferences()
	prefs := s.Current()
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
	s := newDefaultPreferences()
	prefs := s.Current()
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
	s := newDefaultPreferences()
	prefs := s.Current()
	prefs.ShowFooter = true
	_, h := computeLogsViewportSize(100, 10, prefs, "Long subtitle text for testing", "hint")
	if h < 3 {
		t.Fatalf("expected viewport height >= 3, got %d", h)
	}
}

// ---------------------------------------------------------------------------
// modePreferenceLabel
// ---------------------------------------------------------------------------

func TestModePreferenceLabel_Client(t *testing.T) {
	if got := modePreferenceLabel(ModePreferenceClient); got != "client" {
		t.Errorf("got %q, want %q", got, "client")
	}
}

func TestModePreferenceLabel_Server(t *testing.T) {
	if got := modePreferenceLabel(ModePreferenceServer); got != "server" {
		t.Errorf("got %q, want %q", got, "server")
	}
}

func TestModePreferenceLabel_None(t *testing.T) {
	if got := modePreferenceLabel(ModePreferenceNone); got != "not set" {
		t.Errorf("got %q, want %q", got, "not set")
	}
}

func TestModePreferenceLabel_Unknown(t *testing.T) {
	if got := modePreferenceLabel("unknown"); got != "not set" {
		t.Errorf("got %q, want %q", got, "not set")
	}
}

// ---------------------------------------------------------------------------
// uiSettingsRows: serverSupported combinations
// ---------------------------------------------------------------------------

func TestUISettingsRows_NoServer_HasAutoConnectNoModeRow(t *testing.T) {
	prefs := UIPreferences{AutoSelectMode: ModePreferenceNone}
	rows := uiSettingsRows(prefs, false)
	if len(rows) != settingsRowsCount-1 {
		t.Fatalf("expected %d rows, got %d", settingsRowsCount-1, len(rows))
	}
	for _, r := range rows {
		if strings.Contains(r, "Mode") {
			t.Error("Mode row must not appear when serverSupported=false")
		}
	}
	found := false
	for _, r := range rows {
		if strings.Contains(r, "Auto-connect") {
			found = true
		}
	}
	if !found {
		t.Error("Auto-connect row must appear when serverSupported=false")
	}
}

func TestUISettingsRows_ServerSupported_ModeClient_HasModeAndAutoConnect(t *testing.T) {
	prefs := UIPreferences{AutoSelectMode: ModePreferenceClient}
	rows := uiSettingsRows(prefs, true)
	if len(rows) != settingsRowsCount {
		t.Fatalf("expected %d rows, got %d", settingsRowsCount, len(rows))
	}
	hasModeRow, hasAutoConnect := false, false
	for _, r := range rows {
		if strings.Contains(r, "Mode") {
			hasModeRow = true
		}
		if strings.Contains(r, "Auto-connect") {
			hasAutoConnect = true
		}
	}
	if !hasModeRow {
		t.Error("expected Mode row when serverSupported=true")
	}
	if !hasAutoConnect {
		t.Error("expected Auto-connect row when mode=Client")
	}
}

func TestUISettingsRows_ServerSupported_ModeServer_ModeRowNoAutoConnect(t *testing.T) {
	prefs := UIPreferences{AutoSelectMode: ModePreferenceServer}
	rows := uiSettingsRows(prefs, true)
	if len(rows) != settingsRowsCount-1 {
		t.Fatalf("expected %d rows, got %d", settingsRowsCount-1, len(rows))
	}
	hasModeRow := false
	for _, r := range rows {
		if strings.Contains(r, "Mode") {
			hasModeRow = true
		}
		if strings.Contains(r, "Auto-connect") {
			t.Error("Auto-connect row must not appear when mode=Server")
		}
	}
	if !hasModeRow {
		t.Error("expected Mode row when serverSupported=true")
	}
}

func TestUISettingsRows_ServerSupported_ModeNone_ModeRowNoAutoConnect(t *testing.T) {
	prefs := UIPreferences{AutoSelectMode: ModePreferenceNone}
	rows := uiSettingsRows(prefs, true)
	if len(rows) != settingsRowsCount-1 {
		t.Fatalf("expected %d rows, got %d", settingsRowsCount-1, len(rows))
	}
	for _, r := range rows {
		if strings.Contains(r, "Auto-connect") {
			t.Error("Auto-connect row must not appear when mode=None")
		}
	}
}
