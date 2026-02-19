package bubble_tea

import (
	"strings"
	"testing"
	"tungo/infrastructure/telemetry/trafficstats"
)

const (
	ansiColorProfile16 = iota
	ansiColorProfile256
	ansiColorProfileTrueColor
)

func forceANSIColorProfile(t *testing.T, _ int) {
	uiStylesCacheMu.Lock()
	uiStylesCache = map[uiStylesCacheKey]uiStyles{}
	uiStylesCacheMu.Unlock()
	t.Cleanup(func() {
		uiStylesCacheMu.Lock()
		uiStylesCache = map[uiStylesCacheKey]uiStyles{}
		uiStylesCacheMu.Unlock()
	})
}

func Test_computeCardWidth_ClampedToTerminal(t *testing.T) {
	if got := computeCardWidth(40); got > 40 {
		t.Fatalf("card width must not exceed terminal width, got=%d", got)
	}
	if got := computeCardWidth(200); got != maxCardWidth {
		t.Fatalf("expected max width %d, got %d", maxCardWidth, got)
	}
}

func Test_computeCardHeight_ClampedToTerminal(t *testing.T) {
	if got := computeCardHeight(12); got > 12 {
		t.Fatalf("card height must not exceed terminal height, got=%d", got)
	}
	if got := computeCardHeight(200); got != maxCardHeight {
		t.Fatalf("expected max height %d, got %d", maxCardHeight, got)
	}
}

func Test_wrapLine_WrapsLongSentence(t *testing.T) {
	lines := wrapLine("one two three four five", 8)
	if len(lines) < 3 {
		t.Fatalf("expected wrapped output, got %v", lines)
	}
	for _, line := range lines {
		if len([]rune(line)) > 8 {
			t.Fatalf("line exceeds width: %q", line)
		}
	}
}

func Test_wrapLine_BreaksLongWord(t *testing.T) {
	lines := wrapLine("supercalifragilistic", 6)
	if len(lines) < 2 {
		t.Fatalf("expected hard-wrap chunks, got %v", lines)
	}
	for _, line := range lines {
		if len([]rune(line)) > 6 {
			t.Fatalf("line exceeds width: %q", line)
		}
	}
}

func Test_wrapText_ANSIIsNotWrapped(t *testing.T) {
	raw := "\x1b[31mthis should stay as is even if very long\x1b[0m"
	lines := wrapText(raw, 5)
	if len(lines) != 1 {
		t.Fatalf("expected ANSI line to stay intact, got %v", lines)
	}
	if lines[0] != raw {
		t.Fatalf("expected unchanged ANSI line")
	}
}

func Test_formatStatsLines_FixedWidth(t *testing.T) {
	prefs := UIPreferences{StatsUnits: StatsUnitsBiBytes}
	small := trafficstats.Snapshot{
		RXBytesTotal: 1,
		TXBytesTotal: 2,
		RXRate:       3,
		TXRate:       4,
	}
	large := trafficstats.Snapshot{
		RXBytesTotal: 1234567890,
		TXBytesTotal: 987654321,
		RXRate:       12345678,
		TXRate:       9876543,
	}

	smallLines := formatStatsLines(prefs, small)
	largeLines := formatStatsLines(prefs, large)
	if len(smallLines) != 2 || len(largeLines) != 2 {
		t.Fatalf("expected two stats lines")
	}
	if len(smallLines[0]) != len(largeLines[0]) {
		t.Fatalf("expected fixed width line, got %q vs %q", smallLines[0], largeLines[0])
	}
	if len(smallLines[1]) != len(largeLines[1]) {
		t.Fatalf("expected fixed width line, got %q vs %q", smallLines[1], largeLines[1])
	}
}

func TestHelpers_BasicBranches(t *testing.T) {
	if got := formatCount(3, 0); got != "3" {
		t.Fatalf("expected count without max, got %q", got)
	}
	if got := maxInt(1, 5); got != 5 {
		t.Fatalf("expected max=5, got %d", got)
	}
	if unitSystemForPrefs(UIPreferences{StatsUnits: StatsUnitsBytes}) != trafficstats.UnitSystemBytes {
		t.Fatal("expected decimal bytes unit system")
	}
	if unitSystemForPrefs(UIPreferences{StatsUnits: StatsUnitsBiBytes}) != trafficstats.UnitSystemBinary {
		t.Fatal("expected binary unit system")
	}
}

func TestResolveUIStyles_DarkBrandUsesLightBlue(t *testing.T) {
	forceANSIColorProfile(t, ansiColorProfileTrueColor)
	styles := resolveUIStyles(UIPreferences{
		Theme:      ThemeDark,
		StatsUnits: StatsUnitsBiBytes,
		ShowFooter: true,
	})

	if !strings.Contains(styles.brand.prefix, ansiFgBrightCyan) {
		t.Fatalf("expected dark theme brand to use light blue, got %q", styles.brand.prefix)
	}
}

func TestResolveUIStyles_AllDarkThemesKeepBlueBrand(t *testing.T) {
	forceANSIColorProfile(t, ansiColorProfileTrueColor)
	for _, theme := range orderedThemeOptions {
		if !strings.HasPrefix(string(theme), "dark") {
			continue
		}
		styles := resolveUIStyles(UIPreferences{
			Theme:      theme,
			StatsUnits: StatsUnitsBiBytes,
			ShowFooter: true,
		})
		if !strings.Contains(styles.brand.prefix, ansiFgBrightCyan) {
			t.Fatalf("expected dark theme %q to use Go blue brand, got %q", theme, styles.brand.prefix)
		}
	}
}

func TestResolveUIStyles_LightThemeUsesLightBlueAndWarmAccent(t *testing.T) {
	forceANSIColorProfile(t, ansiColorProfileTrueColor)
	styles := resolveUIStyles(UIPreferences{
		Theme:      ThemeLight,
		StatsUnits: StatsUnitsBiBytes,
		ShowFooter: true,
	})

	if !strings.Contains(styles.brand.prefix, ansiFgBrightCyan) {
		t.Fatalf("expected light theme brand to use light blue, got %q", styles.brand.prefix)
	}
	if !strings.Contains(styles.active.prefix, ansiBgBlue) {
		t.Fatalf("expected light theme active accent background, got %q", styles.active.prefix)
	}
}

func TestResolveUIStyles_ThemeSwitchChangesStyles(t *testing.T) {
	forceANSIColorProfile(t, ansiColorProfileTrueColor)
	light := resolveUIStyles(UIPreferences{
		Theme:      ThemeLight,
		StatsUnits: StatsUnitsBiBytes,
		ShowFooter: true,
	})
	contrast := resolveUIStyles(UIPreferences{
		Theme:      ThemeDarkHighContrast,
		StatsUnits: StatsUnitsBiBytes,
		ShowFooter: true,
	})
	if light.active.prefix == contrast.active.prefix {
		t.Fatalf("expected different active style between themes, got light=%q contrast=%q", light.active.prefix, contrast.active.prefix)
	}
	if light.option.prefix == contrast.option.prefix {
		t.Fatalf("expected different option style between themes, got light=%q contrast=%q", light.option.prefix, contrast.option.prefix)
	}
}

func TestResolveUIStyles_DarkVariantsUseDistinctActiveAccent(t *testing.T) {
	forceANSIColorProfile(t, ansiColorProfileTrueColor)

	matrix := resolveUIStyles(UIPreferences{
		Theme:      ThemeDarkMatrix,
		StatsUnits: StatsUnitsBiBytes,
		ShowFooter: true,
	})
	if !strings.Contains(matrix.active.prefix, ansiBgBrightGreen) {
		t.Fatalf("expected dark_matrix active accent background to be bright green, got %q", matrix.active.prefix)
	}

	nord := resolveUIStyles(UIPreferences{
		Theme:      ThemeDarkNord,
		StatsUnits: StatsUnitsBiBytes,
		ShowFooter: true,
	})
	if !strings.Contains(nord.active.prefix, ansiBgCyan) {
		t.Fatalf("expected dark_nord active accent background to be cyan, got %q", nord.active.prefix)
	}
}

func TestWrapAndSplitHelpers_EdgeCases(t *testing.T) {
	if got := wrapBody(nil, 10); got != nil {
		t.Fatalf("expected nil for empty body, got %v", got)
	}
	lines := wrapText("a\nb", 0)
	if len(lines) != 2 || lines[0] != "a" || lines[1] != "b" {
		t.Fatalf("expected split by newline for width<=0, got %v", lines)
	}
	empty := wrapText("", 10)
	if len(empty) != 1 || empty[0] != "" {
		t.Fatalf("expected single empty line, got %v", empty)
	}
	head, tail := splitRunes("abc", 0)
	if head != "" || tail != "abc" {
		t.Fatalf("expected full tail when maxRunes<=0, got head=%q tail=%q", head, tail)
	}
}

func TestContentWidthForTerminal_NonPositiveWidth(t *testing.T) {
	if got := contentWidthForTerminal(0); got != 1 {
		t.Fatalf("expected fallback content width 1, got %d", got)
	}
}

func TestRenderScreen_ANSIAndCanvasFill(t *testing.T) {
	forceANSIColorProfile(t, ansiColorProfileTrueColor)
	UpdateUIPreferences(func(p *UIPreferences) {
		p.Theme = ThemeDark
		p.ShowFooter = true
	})
	t.Cleanup(func() {
		UpdateUIPreferences(func(p *UIPreferences) {
			p.Theme = ThemeLight
			p.ShowFooter = true
		})
	})

	ansiTitle := "\x1b[31mTitle\x1b[0m"
	out := renderScreen(80, 24, ansiTitle, "subtitle", []string{"body"}, "hint")
	if !strings.Contains(out, ansiTitle) {
		t.Fatalf("expected ANSI title preserved, got %q", out)
	}
	if !strings.Contains(out, ansiBgBlack) {
		t.Fatalf("expected dark base background fill ANSI code, got %q", out)
	}
}

func TestBuildFooterBlock_OnlyHintWhenProvided(t *testing.T) {
	styles := resolveUIStyles(UIPreferences{
		Theme:      ThemeLight,
		Language:   "en",
		StatsUnits: StatsUnitsBiBytes,
		ShowFooter: true,
	})
	lines := buildFooterBlock(styles, UIPreferences{
		Theme:      ThemeLight,
		Language:   "en",
		StatsUnits: StatsUnitsBiBytes,
		ShowFooter: true,
	}, 0, "hint line")
	if len(lines) < 2 {
		t.Fatalf("expected footer rule + hint line, got %v", lines)
	}
}

func TestBuildFooterBlock_NoHint_ReturnsNil(t *testing.T) {
	styles := resolveUIStyles(UIPreferences{
		Theme:      ThemeLight,
		Language:   "en",
		StatsUnits: StatsUnitsBiBytes,
		ShowFooter: true,
	})
	lines := buildFooterBlock(styles, UIPreferences{
		Theme:      ThemeLight,
		Language:   "en",
		StatsUnits: StatsUnitsBiBytes,
		ShowFooter: true,
	}, 0, "")
	if lines != nil {
		t.Fatalf("expected nil footer block when hint is empty, got %v", lines)
	}
}

func TestRenderScreen_SubtitleANSIAndNoViewportSize(t *testing.T) {
	UpdateUIPreferences(func(p *UIPreferences) {
		p.Theme = ThemeLight
		p.ShowFooter = false
	})
	out := renderScreen(0, 0, "Title", "\x1b[31mansi subtitle\x1b[0m", []string{"body"}, "")
	if !strings.Contains(out, "ansi subtitle") {
		t.Fatalf("expected subtitle content, got %q", out)
	}
}

func TestComputeCardDimensions_ZeroInput(t *testing.T) {
	if got := computeCardWidth(0); got != 0 {
		t.Fatalf("expected width=0 for zero terminal width, got %d", got)
	}
	if got := computeCardHeight(0); got != 0 {
		t.Fatalf("expected height=0 for zero terminal height, got %d", got)
	}
}

func TestWrapBody_EmptyWrappedLineBranch(t *testing.T) {
	// width<=0 makes wrapText split by '\n'; keep an empty row in the body.
	lines := wrapBody([]string{""}, 0)
	if len(lines) != 1 || lines[0] != "" {
		t.Fatalf("expected preserved empty row, got %v", lines)
	}
}

func TestWrapBody_EmptyWrappedFromHook(t *testing.T) {
	prev := wrapTextForBody
	t.Cleanup(func() { wrapTextForBody = prev })
	wrapTextForBody = func(string, int) []string { return nil }

	lines := wrapBody([]string{"x"}, 10)
	if len(lines) != 1 || lines[0] != "" {
		t.Fatalf("expected fallback empty line for nil wrap result, got %v", lines)
	}
}

func TestWrapLine_WhitespaceOnly(t *testing.T) {
	lines := wrapLine("      ", 2)
	if len(lines) != 1 || lines[0] != "" {
		t.Fatalf("expected whitespace-only line to collapse to empty line, got %v", lines)
	}
}

func TestWrapLine_LongWordFlushesCurrentPrefix(t *testing.T) {
	lines := wrapLine("a supercalifragilistic", 6)
	if len(lines) < 2 {
		t.Fatalf("expected wrapped output, got %v", lines)
	}
	if lines[0] != "a" {
		t.Fatalf("expected current prefix to flush before long word chunks, got %v", lines)
	}
	for _, line := range lines {
		if len([]rune(line)) > 6 {
			t.Fatalf("line exceeds width: %q", line)
		}
	}
}

func TestSplitRunes_NoSplitWhenWithinLimit(t *testing.T) {
	head, tail := splitRunes("abc", 3)
	if head != "abc" || tail != "" {
		t.Fatalf("expected no split, got head=%q tail=%q", head, tail)
	}
}

func TestEnforceBaseThemeFill_ReappliesAfterAnsiReset(t *testing.T) {
	forceANSIColorProfile(t, ansiColorProfileTrueColor)
	out := enforceBaseThemeFill("x"+ansiReset+"y", UIPreferences{Theme: ThemeLight})
	if !strings.Contains(out, ansiReset) {
		t.Fatalf("expected reset sequence in output, got %q", out)
	}
	if !strings.Contains(out, ansiBgBrightWhite) {
		t.Fatalf("expected light background sequence, got %q", out)
	}
}

func TestEnforceBaseThemeFill_NoBaseThemeAvailable(t *testing.T) {
	prev := baseANSIForThemeFunc
	t.Cleanup(func() { baseANSIForThemeFunc = prev })
	baseANSIForThemeFunc = func(UIPreferences) (string, string, bool) {
		return "", "", false
	}

	const input = "plain"
	if out := enforceBaseThemeFill(input, UIPreferences{Theme: ThemeLight}); out != input {
		t.Fatalf("expected unchanged string when base theme is unavailable, got %q", out)
	}
}

func TestVisibleWidthANSI_AndStripANSI_WithCSI(t *testing.T) {
	colored := ansiFgBrightGreen + "TunGo" + ansiReset + " [dev-build]"
	if got := stripANSI(colored); got != "TunGo [dev-build]" {
		t.Fatalf("unexpected stripped value: %q", got)
	}
	if got := visibleWidthANSI(colored); got != len("TunGo [dev-build]") {
		t.Fatalf("unexpected visible width: %d", got)
	}
}

func TestEnforceBaseThemeFill_ReappliesAfterCommonSGRResets(t *testing.T) {
	forceANSIColorProfile(t, ansiColorProfileTrueColor)
	base := ansiBgBlack + ansiFgWhite
	out := enforceBaseThemeFill("\x1b[mx\x1b[49my\x1b[39mz\x1b[0mw", UIPreferences{Theme: ThemeDark})

	if !strings.Contains(out, "\x1b[m"+base) {
		t.Fatalf("expected base reapplied after CSI m reset, got %q", out)
	}
	if !strings.Contains(out, "\x1b[49m"+base) {
		t.Fatalf("expected base reapplied after background reset, got %q", out)
	}
	if !strings.Contains(out, "\x1b[39m"+base) {
		t.Fatalf("expected base reapplied after foreground reset, got %q", out)
	}
	if !strings.Contains(out, "\x1b[0m"+base) {
		t.Fatalf("expected base reapplied after full reset, got %q", out)
	}
}

func TestEnforceBaseThemeFill_AppliesBasePerLine(t *testing.T) {
	forceANSIColorProfile(t, ansiColorProfileTrueColor)
	base := ansiBgBlack + ansiFgWhite
	out := enforceBaseThemeFill("line1\nline2", UIPreferences{Theme: ThemeDark})
	if strings.Count(out, base) < 2 {
		t.Fatalf("expected base sequence on each line, got %q", out)
	}
}

func TestAnsiStylePrefix_UsesAnsiConstants(t *testing.T) {
	prefix := ansiStylePrefix(ansiFgBrightCyan, ansiBgBlack, true)
	if !strings.Contains(prefix, ansiBold) || !strings.Contains(prefix, ansiFgBrightCyan) || !strings.Contains(prefix, ansiBgBlack) {
		t.Fatalf("expected composed ANSI prefix, got %q", prefix)
	}
	if got := ansiStylePrefix("", ansiBgBlack, false); got != "" {
		t.Fatalf("expected empty style when fg is missing, got %q", got)
	}
}
