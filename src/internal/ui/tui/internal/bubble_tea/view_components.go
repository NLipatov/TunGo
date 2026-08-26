package bubble_tea

import (
	"strconv"
	"strings"
	"time"
	tuiconfig "tungo/internal/config/tui"
	"unicode"
)

const runtimeLogViewportSnapshotLimit = 4096

var selectorTabs = [...]string{"Main", "Settings", "Logs"}
var runtimeTabs = [...]string{"Dataplane", "Settings", "Logs"}

func renderTabsLine(
	productLabel string,
	tabs []string,
	activeIndex int,
	contentWidth int,
	styles uiStyles,
) string {
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

	var rendered string
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

func uiSettingsRows(prefs tuiconfig.Configuration, serverSupported bool) []string {
	rows := []string{
		"Theme      : " + strings.ToUpper(strings.ReplaceAll(string(prefs.Theme), "_", " ")),
		"Stats units: " + statsUnitsLabel(prefs.StatsUnits),
		"Dataplane stats: " + onOff(prefs.ShowDataplaneStats),
		"Dataplane graph: " + onOff(prefs.ShowDataplaneGraph),
		"Show footer: " + onOff(prefs.ShowFooter),
	}
	if serverSupported {
		rows = append(rows, "Autoselect Mode: "+modePreferenceLabel(prefs.AutoSelectMode))
	}
	if prefs.AutoSelectMode == tuiconfig.ModePreferenceClient || !serverSupported {
		rows = append(rows, "Auto-connect: "+onOff(prefs.AutoConnect))
	}
	return rows
}

func onOff(value bool) string {
	if value {
		return "ON"
	}
	return "OFF"
}

func modePreferenceLabel(m tuiconfig.ModePreference) string {
	switch m {
	case tuiconfig.ModePreferenceClient:
		return "client"
	case tuiconfig.ModePreferenceServer:
		return "server"
	default:
		return "not set"
	}
}

func statsUnitsLabel(units tuiconfig.StatsUnits) string {
	if units == tuiconfig.StatsUnitsBytes {
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

func renderLogsViewportLines(lines []string, width int, styles uiStyles) []string {
	if len(lines) == 0 {
		return []string{"No logs yet"}
	}

	rows := make([]string, 0, len(lines)*2)
	lastDate := -1
	for i, line := range lines {
		if i > 0 {
			rows = append(rows, "")
		}

		timestamp, level, message, ok := parseSlogTextLine(line)
		if !ok {
			rows = append(rows, wrapText(line, width)...)
			continue
		}

		date := timestamp.Year()*1000 + timestamp.YearDay()
		if date != lastDate {
			formattedDate := timestamp.Format("2006-01-02")
			rows = append(rows, styles.meta.Render(renderLogDateDivider(formattedDate, width)))
			lastDate = date
		}
		rows = appendLogEntry(rows, timestamp, level, message, width)
	}
	return rows
}

func parseSlogTextLine(line string) (time.Time, string, string, bool) {
	timeField, rest, ok := strings.Cut(line, " ")
	if !ok || !strings.HasPrefix(timeField, "time=") {
		return time.Time{}, "", "", false
	}
	timestamp, err := time.Parse(time.RFC3339Nano, strings.TrimPrefix(timeField, "time="))
	if err != nil {
		return time.Time{}, "", "", false
	}

	levelField, messageField, ok := strings.Cut(rest, " ")
	if !ok || !strings.HasPrefix(levelField, "level=") || !strings.HasPrefix(messageField, "msg=") {
		return time.Time{}, "", "", false
	}

	level := strings.TrimPrefix(levelField, "level=")
	message := displaySlogMessage(strings.TrimPrefix(messageField, "msg="))
	return timestamp, level, message, true
}

func displaySlogMessage(valueAndAttrs string) string {
	if len(valueAndAttrs) < 2 || valueAndAttrs[0] != '"' {
		return valueAndAttrs
	}

	quoted, err := strconv.QuotedPrefix(valueAndAttrs)
	if err != nil {
		return valueAndAttrs
	}
	message, err := strconv.Unquote(quoted)
	if err != nil {
		return valueAndAttrs
	}
	return sanitizeLogMessage(message) + valueAndAttrs[len(quoted):]
}

func sanitizeLogMessage(message string) string {
	if strings.IndexFunc(message, unicode.IsControl) < 0 {
		return message
	}

	var sanitized strings.Builder
	sanitized.Grow(len(message))
	for _, r := range message {
		switch {
		case r == '\n':
			sanitized.WriteRune(r)
		case r == '\t':
			sanitized.WriteString("    ")
		case unicode.IsControl(r):
			quoted := strconv.QuoteRune(r)
			sanitized.WriteString(quoted[1 : len(quoted)-1])
		default:
			sanitized.WriteRune(r)
		}
	}
	return sanitized.String()
}

func renderLogDateDivider(date string, width int) string {
	if width <= len(date)+2 {
		return truncateWithEllipsis(date, width)
	}

	ruleWidth := width - len(date) - 2
	leftWidth := ruleWidth / 2
	return strings.Repeat("─", leftWidth) + " " + date + " " + strings.Repeat("─", ruleWidth-leftWidth)
}

func appendLogEntry(rows []string, timestamp time.Time, level, message string, width int) []string {
	marker := renderLogLevelMarker(level)
	formattedTime := timestamp.Format("15:04:05.000")
	prefix := formattedTime + " " + padRightVisible(level, 5) + " " + marker + " "
	prefixWidth := visibleWidthANSI(prefix)
	if width <= 0 {
		return append(rows, stripANSI(prefix)+message)
	}
	if width <= prefixWidth {
		rows = append(rows, wrapText(formattedTime+" "+level, width)...)
		messagePrefix := marker + " "
		messageWidth := width - 2
		if messageWidth < 1 {
			messagePrefix = marker
			messageWidth = width - 1
		}
		if messageWidth < 1 {
			for _, line := range wrapText(message, width) {
				rows = append(rows, marker, line)
			}
			return rows
		}
		for _, line := range wrapText(message, messageWidth) {
			rows = append(rows, messagePrefix+line)
		}
		return rows
	}

	messageLines := wrapText(message, width-prefixWidth)
	rows = append(rows, prefix+messageLines[0])
	if len(messageLines) == 1 {
		return rows
	}
	continuationPrefix := strings.Repeat(" ", prefixWidth-2) + marker + " "
	for i := 1; i < len(messageLines); i++ {
		rows = append(rows, continuationPrefix+messageLines[i])
	}
	return rows
}

func renderLogLevelMarker(level string) string {
	upperLevel := strings.ToUpper(level)
	color := ""
	switch {
	case strings.HasPrefix(upperLevel, "TRACE"), strings.HasPrefix(upperLevel, "DEBUG"):
		color = ansiFgBrightBlack
	case strings.HasPrefix(upperLevel, "INFO"):
		color = ansiFgBrightGreen
	case strings.HasPrefix(upperLevel, "WARN"):
		color = ansiFgBrightYellow
	case strings.HasPrefix(upperLevel, "ERROR"), strings.HasPrefix(upperLevel, "FATAL"):
		color = ansiFgBrightRed
	default:
		return "│"
	}
	return color + "│" + ansiReset
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
	prefs tuiconfig.Configuration,
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
