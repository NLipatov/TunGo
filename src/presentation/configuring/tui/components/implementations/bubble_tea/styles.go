package bubble_tea

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"tungo/infrastructure/telemetry/trafficstats"

	"github.com/charmbracelet/lipgloss"
)

const (
	minCardWidth    = 36
	maxCardWidth    = 96
	sideInsetCols   = 6
	minCardHeight   = 16
	maxCardHeight   = 30
	topBottomInsets = 4
	statsValueWidth = 12
)

type uiStyles struct {
	screenFrame lipgloss.Style
	headerBar   lipgloss.Style
	headerTitle lipgloss.Style
	headerRule  lipgloss.Style
	title       lipgloss.Style
	subtitle    lipgloss.Style
	hint        lipgloss.Style
	option      lipgloss.Style
	active      lipgloss.Style
	inputFrame  lipgloss.Style
	meta        lipgloss.Style
	canvas      lipgloss.Style
}

const ansiReset = "\x1b[0m"

var (
	statsFooterFormatter = formatStatsFooter
	wrapTextForBody      = wrapText
	baseANSIForThemeFunc = baseANSIForTheme
)

func themeColorForPrefs(prefs UIPreferences, light, dark string) lipgloss.TerminalColor {
	if prefs.Theme == ThemeLight {
		return lipgloss.Color(light)
	}
	return lipgloss.Color(dark)
}

func themeColor(light, dark string) lipgloss.TerminalColor {
	return themeColorForPrefs(CurrentUIPreferences(), light, dark)
}

type palette struct {
	backgroundLight string
	backgroundDark  string
	textLight       string
	textDark        string
	mutedLight      string
	mutedDark       string
	accent          string
	activeText      string
}

func paletteForPrefs(prefs UIPreferences) palette {
	_ = prefs
	return palette{
		backgroundLight: "#ffffff",
		backgroundDark:  "#000000",
		textLight:       "#000000",
		textDark:        "#00ff66",
		mutedLight:      "#374151",
		mutedDark:       "#5fd18a",
		accent:          "#00ADD8",
		activeText:      "#000000",
	}
}

func resolveUIStyles(prefs UIPreferences) uiStyles {
	p := paletteForPrefs(prefs)

	textColor := themeColorForPrefs(prefs, p.textLight, p.textDark)
	mutedColor := themeColorForPrefs(prefs, p.mutedLight, p.mutedDark)
	accentColor := themeColorForPrefs(prefs, p.accent, p.accent)
	activeTextColor := themeColorForPrefs(prefs, p.activeText, p.activeText)

	backgroundColor := themeColorForPrefs(prefs, p.backgroundLight, p.backgroundDark)

	screenFrameStyle := lipgloss.NewStyle().
		Border(lipgloss.ASCIIBorder()).
		BorderForeground(accentColor).
		Foreground(textColor).
		Padding(1, 2)
	inputFrameStyle := lipgloss.NewStyle().
		Border(lipgloss.ASCIIBorder()).
		BorderForeground(accentColor).
		Foreground(textColor).
		Padding(0, 1)
	canvasStyle := lipgloss.NewStyle().
		Foreground(textColor)
	screenFrameStyle = screenFrameStyle.Background(backgroundColor)
	inputFrameStyle = inputFrameStyle.Background(backgroundColor)
	canvasStyle = canvasStyle.Background(backgroundColor)

	return uiStyles{
		screenFrame: screenFrameStyle,
		headerBar: lipgloss.NewStyle().
			Foreground(textColor).
			Padding(0, 1),
		headerTitle: lipgloss.NewStyle().
			Bold(true).
			Foreground(textColor),
		headerRule: lipgloss.NewStyle().
			Foreground(accentColor),
		title: lipgloss.NewStyle().
			Bold(true).
			Foreground(textColor),
		subtitle: lipgloss.NewStyle().
			Foreground(mutedColor),
		hint: lipgloss.NewStyle().
			Foreground(mutedColor),
		option: lipgloss.NewStyle().
			Foreground(textColor),
		active: lipgloss.NewStyle().
			Bold(true).
			Foreground(activeTextColor).
			Background(accentColor),
		inputFrame: inputFrameStyle,
		meta: lipgloss.NewStyle().
			Foreground(mutedColor),
		canvas: canvasStyle,
	}
}

func optionTextStyle() lipgloss.Style {
	return resolveUIStyles(CurrentUIPreferences()).option
}

func activeOptionTextStyle() lipgloss.Style {
	return resolveUIStyles(CurrentUIPreferences()).active
}

func inputContainerStyle() lipgloss.Style {
	return resolveUIStyles(CurrentUIPreferences()).inputFrame
}

func metaTextStyle() lipgloss.Style {
	return resolveUIStyles(CurrentUIPreferences()).meta
}

func headerLabelStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(themeColor("#00ADD8", "#00ADD8"))
}

func renderScreen(width, height int, title, subtitle string, body []string, hint string) string {
	prefs := CurrentUIPreferences()
	styles := resolveUIStyles(prefs)

	frameStyle := styles.screenFrame
	targetWidth := 0
	contentWidth := 0
	targetHeight := 0
	contentHeight := 0
	if width > 0 {
		targetWidth = computeCardWidth(width)
		contentWidth = maxInt(1, targetWidth-frameStyle.GetHorizontalFrameSize())
	}
	if height > 0 {
		targetHeight = computeCardHeight(height)
		contentHeight = maxInt(1, targetHeight-frameStyle.GetVerticalFrameSize())
	}

	mainLines := make([]string, 0, len(body)+8)
	if strings.TrimSpace(title) != "" {
		headerTitle := title
		if !containsANSI(title) {
			headerTitle = styles.headerTitle.Render(title)
		}
		if contentWidth > 0 {
			mainLines = append(mainLines, styles.headerBar.Width(contentWidth).Render(headerTitle))
			mainLines = append(mainLines, styles.headerRule.Render(strings.Repeat("-", maxInt(1, contentWidth))))
		} else {
			mainLines = append(mainLines, styles.headerBar.Render(headerTitle))
			mainLines = append(mainLines, styles.headerRule.Render("-"))
		}
		mainLines = append(mainLines, "")
	}
	if strings.TrimSpace(subtitle) != "" {
		for _, line := range wrapText(subtitle, contentWidth) {
			if containsANSI(line) {
				mainLines = append(mainLines, line)
				continue
			}
			mainLines = append(mainLines, styles.subtitle.Render(line))
		}
		mainLines = append(mainLines, "")
	}

	mainLines = append(mainLines, wrapBody(body, contentWidth)...)

	footerLines := []string{}
	if prefs.ShowFooter {
		footerLines = buildFooterBlock(styles, prefs, contentWidth, hint)
	}

	contentLines := mainLines
	if contentHeight > 0 {
		required := len(mainLines) + len(footerLines)
		if required < contentHeight {
			contentLines = append(contentLines, make([]string, contentHeight-required)...)
		}
	}
	contentLines = append(contentLines, footerLines...)

	card := frameStyle.Render(lipgloss.JoinVertical(lipgloss.Left, contentLines...))
	if width > 0 && height > 0 {
		options := []lipgloss.WhitespaceOption{
			lipgloss.WithWhitespaceForeground(styles.canvas.GetForeground()),
		}
		if styles.canvas.GetBackground() != nil {
			options = append(options, lipgloss.WithWhitespaceBackground(styles.canvas.GetBackground()))
		}
		rendered := styles.canvas.Render(lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, card, options...))
		return enforceBaseThemeFill(rendered, prefs)
	}
	return enforceBaseThemeFill(styles.canvas.Render(card), prefs) + "\n"
}

func buildFooterBlock(styles uiStyles, prefs UIPreferences, contentWidth int, hint string) []string {
	hintLines := []string{}
	if strings.TrimSpace(hint) != "" {
		for _, line := range wrapText(hint, contentWidth) {
			hintLines = append(hintLines, styles.hint.Render(line))
		}
	}

	statsLines := []string{}
	for _, metricLine := range statsFooterFormatter(prefs) {
		line := styles.meta.Render(metricLine)
		if contentWidth > 0 {
			line = lipgloss.NewStyle().Width(contentWidth).Align(lipgloss.Right).Render(line)
		}
		statsLines = append(statsLines, line)
	}
	if len(hintLines) == 0 && len(statsLines) == 0 {
		return nil
	}

	rule := styles.headerRule.Render("-")
	if contentWidth > 0 {
		rule = styles.headerRule.Render(strings.Repeat("-", maxInt(1, contentWidth)))
	}

	block := make([]string, 0, len(hintLines)+len(statsLines)+4)
	block = append(block, rule)
	if len(hintLines) > 0 {
		block = append(block, hintLines...)
	}
	if len(hintLines) > 0 && len(statsLines) > 0 {
		block = append(block, "")
	}
	if len(statsLines) > 0 {
		block = append(block, statsLines...)
	}
	return block
}

func formatStatsFooter(prefs UIPreferences) []string {
	snapshot := trafficstats.SnapshotGlobal()
	return formatStatsLines(prefs, snapshot)
}

func formatStatsLines(prefs UIPreferences, snapshot trafficstats.Snapshot) []string {
	rxRate := formatRateForPrefs(prefs, snapshot.RXRate)
	txRate := formatRateForPrefs(prefs, snapshot.TXRate)
	rxTotal := formatTotalForPrefs(prefs, snapshot.RXBytesTotal)
	txTotal := formatTotalForPrefs(prefs, snapshot.TXBytesTotal)

	return []string{
		fmt.Sprintf("%-8s %*s | %-8s %*s", "RX", statsValueWidth, rxRate, "TX", statsValueWidth, txRate),
		fmt.Sprintf("%-8s %*s | %-8s %*s", "Total RX", statsValueWidth, rxTotal, "Total TX", statsValueWidth, txTotal),
	}
}

func unitSystemForPrefs(prefs UIPreferences) trafficstats.UnitSystem {
	if prefs.StatsUnits == StatsUnitsBytes {
		return trafficstats.UnitSystemBytes
	}
	return trafficstats.UnitSystemBinary
}

func formatRateForPrefs(prefs UIPreferences, bytesPerSecond uint64) string {
	return trafficstats.FormatRateWithSystem(bytesPerSecond, unitSystemForPrefs(prefs))
}

func formatTotalForPrefs(prefs UIPreferences, bytes uint64) string {
	return trafficstats.FormatTotalWithSystem(bytes, unitSystemForPrefs(prefs))
}

func formatCount(current, max int) string {
	if max > 0 {
		return fmt.Sprintf("%d/%d", current, max)
	}
	return fmt.Sprintf("%d", current)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func computeCardWidth(terminalWidth int) int {
	if terminalWidth <= 0 {
		return 0
	}
	available := terminalWidth - sideInsetCols
	if available < minCardWidth {
		available = terminalWidth - 2
	}
	available = maxInt(1, available)
	return minInt(maxCardWidth, minInt(available, terminalWidth))
}

func computeCardHeight(terminalHeight int) int {
	if terminalHeight <= 0 {
		return 0
	}
	available := terminalHeight - topBottomInsets
	if available < minCardHeight {
		available = terminalHeight - 2
	}
	available = maxInt(1, available)
	return minInt(maxCardHeight, minInt(available, terminalHeight))
}

func contentWidthForTerminal(terminalWidth int) int {
	if terminalWidth <= 0 {
		return 1
	}
	styles := resolveUIStyles(CurrentUIPreferences())
	cardWidth := computeCardWidth(terminalWidth)
	return maxInt(1, cardWidth-styles.screenFrame.GetHorizontalFrameSize())
}

func wrapBody(lines []string, width int) []string {
	if len(lines) == 0 {
		return nil
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		wrapped := wrapTextForBody(line, width)
		if len(wrapped) == 0 {
			out = append(out, "")
			continue
		}
		out = append(out, wrapped...)
	}
	return out
}

func wrapText(s string, width int) []string {
	if s == "" {
		return []string{""}
	}
	if width <= 0 || containsANSI(s) {
		return strings.Split(s, "\n")
	}

	parts := strings.Split(s, "\n")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		out = append(out, wrapLine(part, width)...)
	}
	return out
}

func wrapLine(line string, width int) []string {
	if width <= 0 || utf8.RuneCountInString(line) <= width {
		return []string{line}
	}

	words := strings.Fields(line)
	if len(words) == 0 {
		return []string{""}
	}

	var out []string
	current := ""
	for _, word := range words {
		for utf8.RuneCountInString(word) > width {
			if current != "" {
				out = append(out, current)
				current = ""
			}
			chunk, rest := splitRunes(word, width)
			out = append(out, chunk)
			word = rest
		}

		if current == "" {
			current = word
			continue
		}
		next := current + " " + word
		if utf8.RuneCountInString(next) <= width {
			current = next
			continue
		}

		out = append(out, current)
		current = word
	}
	if current != "" {
		out = append(out, current)
	}

	return out
}

func splitRunes(s string, maxRunes int) (head, tail string) {
	if maxRunes <= 0 {
		return "", s
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s, ""
	}
	return string(runes[:maxRunes]), string(runes[maxRunes:])
}

func containsANSI(s string) bool {
	return strings.Contains(s, "\x1b[")
}

func enforceBaseThemeFill(s string, prefs UIPreferences) string {
	bg, fg, ok := baseANSIForThemeFunc(prefs)
	if !ok {
		return s
	}
	base := bg + fg
	reapplied := strings.ReplaceAll(s, ansiReset, ansiReset+base)
	return base + reapplied + ansiReset
}

func baseANSIForTheme(prefs UIPreferences) (bg string, fg string, ok bool) {
	if prefs.Theme == ThemeLight {
		// White canvas + black default text.
		return "\x1b[48;2;255;255;255m", "\x1b[38;2;0;0;0m", true
	}
	// Black canvas + green default text.
	return "\x1b[48;2;0;0;0m", "\x1b[38;2;0;255;102m", true
}
