package bubble_tea

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func renderTabsLine(productLabel string, tabs []string, activeIndex int) string {
	label := headerLabelStyle().Render(productLabel)
	renderedTabs := make([]string, 0, len(tabs))
	for i, tab := range tabs {
		caption := fmt.Sprintf(" %s ", strings.TrimSpace(tab))
		if i == activeIndex {
			renderedTabs = append(renderedTabs, activeOptionTextStyle().Render(caption))
			continue
		}
		renderedTabs = append(renderedTabs, optionTextStyle().Render(caption))
	}
	return lipgloss.JoinHorizontal(
		lipgloss.Left,
		label,
		"  ",
		strings.Join(renderedTabs, " "),
	)
}

func renderSelectableRows(rows []string, cursor int, width int) []string {
	out := make([]string, 0, len(rows))
	for i, row := range rows {
		prefix := "  "
		lineStyle := optionTextStyle()
		if i == cursor {
			prefix = "> "
			lineStyle = activeOptionTextStyle()
		}
		line := truncateWithEllipsis(prefix+row, width)
		out = append(out, lineStyle.Render(line))
	}
	return out
}

func uiSettingsRows(prefs UIPreferences) []string {
	return []string{
		fmt.Sprintf("Theme      : %s", strings.ToUpper(string(prefs.Theme))),
		fmt.Sprintf("Language   : %s", strings.ToUpper(prefs.Language)),
		fmt.Sprintf("Stats units: %s", strings.ToUpper(string(prefs.StatsUnits))),
		fmt.Sprintf("Show footer: %s", onOff(prefs.ShowFooter)),
	}
}

func renderLogsBody(lines []string, width int) []string {
	if len(lines) == 0 {
		return []string{metaTextStyle().Render("  No logs yet")}
	}
	body := make([]string, 0, len(lines))
	for _, line := range lines {
		row := truncateWithEllipsis("  "+line, width)
		body = append(body, metaTextStyle().Render(row))
	}
	return body
}

func truncateWithEllipsis(s string, width int) string {
	if width <= 0 {
		return s
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

func logTailLimit(height int) int {
	limit := 8
	if height > 0 {
		limit = maxInt(4, minInt(14, height/3))
	}
	return limit
}
