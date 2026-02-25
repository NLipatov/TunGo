package bubble_tea

import (
	"strings"
	"sync"
)

const runtimeLogViewportSnapshotLimit = 4096

var (
	selectorTabs  = [...]string{"Main", "Settings", "Logs"}
	runtimeTabs   = [...]string{"Dataplane", "Settings", "Logs"}
	tabsLineCache sync.Map
)

type tabsLineCacheKey struct {
	tabsID       string
	productName  string
	theme        ThemeOption
	activeIndex  int
	contentWidth int
}

func renderTabsLine(
	productLabel string,
	tabsID string,
	tabs []string,
	activeIndex int,
	contentWidth int,
	theme ThemeOption,
	styles uiStyles,
) string {
	cacheKey := tabsLineCacheKey{
		tabsID:       tabsID,
		productName:  productLabel,
		theme:        theme,
		activeIndex:  activeIndex,
		contentWidth: contentWidth,
	}
	if cached, ok := tabsLineCache.Load(cacheKey); ok {
		return cached.(string)
	}

	var tabsOut strings.Builder
	tabsOut.Grow(len(productLabel) + len(tabs)*16)
	for i, tab := range tabs {
		if i > 0 {
			tabsOut.WriteByte(' ')
		}
		caption := " " + strings.TrimSpace(tab) + " "
		if i == activeIndex {
			tabsOut.WriteString(styles.active.Render(caption))
			continue
		}
		tabsOut.WriteString(styles.option.Render(caption))
	}
	left := tabsOut.String()
	right := styles.brand.Render(productLabel)

	rendered := left
	if contentWidth > 1 {
		leftWidth := visibleWidthANSI(left)
		rightWidth := visibleWidthANSI(right)
		gap := contentWidth - leftWidth - rightWidth
		if gap >= 1 {
			var out strings.Builder
			out.Grow(len(left) + len(right) + gap)
			out.WriteString(left)
			out.WriteString(strings.Repeat(" ", gap))
			out.WriteString(right)
			rendered = out.String()
		} else {
			// Keep product label visible and accent-colored even on narrow widths.
			rendered = right
		}
	} else {
		var out strings.Builder
		out.Grow(len(left) + len(right) + 2)
		out.WriteString(left)
		out.WriteString("  ")
		out.WriteString(right)
		rendered = out.String()
	}
	tabsLineCache.Store(cacheKey, rendered)
	return rendered
}

func renderSelectableRows(rows []string, cursor int, width int, styles uiStyles) []string {
	out := make([]string, 0, len(rows))
	for i, row := range rows {
		prefix := "  "
		if i == cursor {
			prefix = "> "
		}
		line := truncateWithEllipsis(prefix+row, width)
		if i == cursor {
			out = append(out, styles.active.Render(line))
			continue
		}
		out = append(out, line)
	}
	return out
}

func uiSettingsRows(prefs UIPreferences) []string {
	return []string{
		"Theme      : " + strings.ToUpper(strings.ReplaceAll(string(prefs.Theme), "_", " ")),
		"Stats units: " + statsUnitsLabel(prefs.StatsUnits),
		"Dataplane stats: " + onOff(prefs.ShowDataplaneStats),
		"Dataplane graph: " + onOff(prefs.ShowDataplaneGraph),
		"Show footer: " + onOff(prefs.ShowFooter),
	}
}

func statsUnitsLabel(units StatsUnitsOption) string {
	if units == StatsUnitsBytes {
		return "Decimal units (KB/MB/GB)"
	}
	return "Binary units (KiB/MiB/GiB)"
}

func renderLogsBody(lines []string, width int, styles uiStyles) []string {
	if len(lines) == 0 {
		return []string{styles.meta.Render("  No logs yet")}
	}
	body := make([]string, 0, len(lines))
	for _, line := range lines {
		row := truncateWithEllipsis("  "+line, width)
		body = append(body, styles.meta.Render(row))
	}
	return body
}

func renderLogsViewportContent(lines []string, width int, styles uiStyles) string {
	_ = styles
	if len(lines) == 0 {
		return "No logs yet"
	}
	var body strings.Builder
	body.Grow(len(lines) * (width + 1))
	for i, line := range lines {
		if i > 0 {
			body.WriteByte('\n')
		}
		body.WriteString(truncateWithEllipsis(line, width))
	}
	return body.String()
}

func runtimeLogSnapshot(feed RuntimeLogFeed, reusable *[]string) []string {
	if feed == nil {
		return nil
	}
	if reusable == nil {
		return feed.Tail(runtimeLogViewportSnapshotLimit)
	}
	if cap(*reusable) < runtimeLogViewportSnapshotLimit {
		*reusable = make([]string, runtimeLogViewportSnapshotLimit)
	}
	buf := (*reusable)[:runtimeLogViewportSnapshotLimit]
	n := feed.TailInto(buf, runtimeLogViewportSnapshotLimit)
	if n <= 0 {
		return nil
	}
	return buf[:n]
}

func computeLogsViewportSize(
	terminalWidth, terminalHeight int,
	prefs UIPreferences,
	subtitle, hint string,
) (contentWidth int, viewportHeight int) {
	contentWidth = 80
	if terminalWidth > 0 {
		contentWidth = contentWidthForTerminal(terminalWidth)
	}
	if terminalHeight <= 0 {
		return contentWidth, 8
	}

	styles := resolveUIStyles(prefs)
	contentHeight := maxInt(1, computeCardHeight(terminalHeight)-frameVertSize)

	used := 3 // header tabs row + rule + spacing
	if strings.TrimSpace(subtitle) != "" {
		used += len(wrapText(subtitle, contentWidth)) + 1
	}
	if prefs.ShowFooter {
		used += len(buildFooterBlock(styles, prefs, contentWidth, hint))
	}

	viewportHeight = contentHeight - used
	if viewportHeight < 3 {
		viewportHeight = 3
	}
	return contentWidth, viewportHeight
}

func truncateWithEllipsis(s string, width int) string {
	if width <= 0 {
		return s
	}
	if !containsANSI(s) && isASCIIString(s) {
		if len(s) <= width {
			return s
		}
		if width <= 3 {
			return s[:width]
		}
		return s[:width-3] + "..."
	}
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	if width <= 3 {
		return string(runes[:width])
	}
	return string(runes[:width-3]) + "..."
}

func isASCIIString(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func logTailLimit(height int) int {
	limit := 8
	if height > 0 {
		limit = maxInt(4, minInt(14, height/3))
	}
	return limit
}
