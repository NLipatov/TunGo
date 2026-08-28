package bubble_tea

import (
	"strings"
	tuiconfig "tungo/internal/config/tui"
	"unicode/utf8"
)

const (
	minCardWidth    = 36
	maxCardWidth    = 96
	sideInsetCols   = 6
	minCardHeight   = 16
	maxCardHeight   = 30
	topBottomInsets = 4
	framePadX       = 2
	framePadY       = 1
	framePadStr     = "  " // must match framePadX
	frameBorderX    = 2
	frameBorderY    = 2
	frameHorizSize  = frameBorderX + framePadX*2 // 6
	frameVertSize   = frameBorderY + framePadY*2 // 4
)

// renderScreen renders a screen with wrapped body text and an optional footer hint.
func renderScreen(width, height int, title, subtitle string, body []string, hint string, prefs tuiconfig.Configuration, styles uiStyles) string {
	return renderScreenWithBodyMode(width, height, title, subtitle, body, hint, true, prefs, styles)
}

// renderScreenRaw renders a screen using the provided body lines without wrapping them.
func renderScreenRaw(width, height int, title, subtitle string, body []string, hint string, prefs tuiconfig.Configuration, styles uiStyles) string {
	return renderScreenWithBodyMode(width, height, title, subtitle, body, hint, false, prefs, styles)
}

// renderScreenWithBodyMode renders a styled card containing the title, subtitle, body,
// and optional footer within the requested terminal dimensions.
// Body lines can be wrapped according to the selected mode, and the card is centered
// when both terminal dimensions are provided.
// The returned string contains the rendered card with the configured base theme.
// @param? Go doesn't use tags. Need Godoc convention no @param. We should perhaps no params. But "returned string" okay, summary not starts Returns. Yet requirement return documentation applicable but Go docs typically sentence. Could say "It returns..." within comment. Better. Also function name starts. final only comment. Ensure not claim optional dimensions maybe yes. "zero or negative dimensions" behavior base card newline, but not necessary. Use two lines.
func renderScreenWithBodyMode(
	width, height int,
	title, subtitle string,
	body []string,
	hint string,
	wrapBodyLines bool,
	prefs tuiconfig.Configuration,
	styles uiStyles,
) string {
	targetWidth := 0
	contentWidth := 0
	targetHeight := 0
	contentHeight := 0
	if width > 0 {
		targetWidth = computeCardWidth(width)
		contentWidth = maxInt(1, targetWidth-frameHorizSize)
	}
	if height > 0 {
		targetHeight = computeCardHeight(height)
		contentHeight = maxInt(1, targetHeight-frameVertSize)
	}

	mainLines := make([]string, 0, len(body)+8)
	if strings.TrimSpace(title) != "" {
		headerTitle := title
		if !containsANSI(title) {
			headerTitle = styles.headerTitle.Render(title)
		}
		if contentWidth > 0 {
			mainLines = append(mainLines, styles.headerBar.Render(padRightVisible(headerTitle, contentWidth)))
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

	if wrapBodyLines {
		mainLines = appendWrappedBody(mainLines, body, contentWidth)
	} else {
		mainLines = append(mainLines, body...)
	}

	var footerLines []string
	if prefs.ShowFooter {
		footerLines = buildFooterBlock(styles, prefs, contentWidth, hint)
	}

	contentLines := mainLines
	if contentHeight > 0 {
		required := len(contentLines) + len(footerLines)
		for required < contentHeight {
			contentLines = append(contentLines, "")
			required++
		}
	}
	contentLines = append(contentLines, footerLines...)

	card := buildASCIICard(contentLines, contentWidth)
	if width > 0 && height > 0 {
		placed := placeCardCentered(card, width, height)
		return enforceBaseThemeFill(placed, prefs)
	}
	return enforceBaseThemeFill(card, prefs) + "\n"
}

// buildFooterBlock builds a styled footer containing a horizontal rule and wrapped hint text.
// It returns nil when the hint is blank.
func buildFooterBlock(styles uiStyles, prefs tuiconfig.Configuration, contentWidth int, hint string) []string {
	_ = prefs
	var hintLines []string
	if strings.TrimSpace(hint) != "" {
		for _, line := range wrapText(hint, contentWidth) {
			hintLines = append(hintLines, styles.hint.Render(line))
		}
	}

	if len(hintLines) == 0 {
		return nil
	}

	rule := styles.headerRule.Render("-")
	if contentWidth > 0 {
		rule = styles.headerRule.Render(strings.Repeat("-", maxInt(1, contentWidth)))
	}

	block := make([]string, 0, len(hintLines)+1)
	block = append(block, rule)
	block = append(block, hintLines...)
	return block
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
	cardWidth := computeCardWidth(terminalWidth)
	return maxInt(1, cardWidth-frameHorizSize)
}

func wrapBody(lines []string, width int) []string {
	if len(lines) == 0 {
		return nil
	}
	return appendWrappedBody(nil, lines, width)
}

func appendWrappedBody(dst []string, lines []string, width int) []string {
	if len(lines) == 0 {
		return dst
	}
	out := dst
	for _, line := range lines {
		if line == "" {
			out = append(out, "")
			continue
		}
		if (width <= 0 || containsANSI(line)) && !strings.Contains(line, "\n") {
			out = append(out, line)
			continue
		}
		out = append(out, wrapText(line, width)...)
	}
	return out
}

func wrapText(s string, width int) []string {
	if s == "" {
		return []string{""}
	}
	if width <= 0 || containsANSI(s) {
		if !strings.Contains(s, "\n") {
			return []string{s}
		}
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
	currentLen := 0
	for _, word := range words {
		for utf8.RuneCountInString(word) > width {
			if currentLen > 0 {
				out = append(out, current)
				current = ""
				currentLen = 0
			}
			chunk, rest := splitRunes(word, width)
			out = append(out, chunk)
			word = rest
		}

		wordLen := utf8.RuneCountInString(word)
		if currentLen == 0 {
			current = word
			currentLen = wordLen
			continue
		}
		if currentLen+1+wordLen <= width {
			current = current + " " + word
			currentLen += 1 + wordLen
			continue
		}

		out = append(out, current)
		current = word
		currentLen = wordLen
	}
	if currentLen > 0 {
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

func buildASCIICard(contentLines []string, contentWidth int) string {
	lines := contentLines
	if lines == nil {
		lines = []string{}
	}
	effectiveWidth := contentWidth
	if effectiveWidth <= 0 {
		effectiveWidth = 1
		for _, line := range lines {
			if w := visibleWidthANSI(line); w > effectiveWidth {
				effectiveWidth = w
			}
		}
	}
	innerWidth := effectiveWidth + framePadX*2
	topBottom := "+" + strings.Repeat("-", innerWidth) + "+"
	paddingLine := "|" + strings.Repeat(" ", innerWidth) + "|"

	var out strings.Builder
	estimatedLines := len(lines) + frameVertSize
	out.Grow(estimatedLines * (innerWidth + frameBorderX + 1))

	out.WriteString(topBottom)
	out.WriteByte('\n')
	out.WriteString(paddingLine)
	out.WriteByte('\n')
	for _, line := range lines {
		out.WriteByte('|')
		out.WriteString(framePadStr)
		out.WriteString(line)
		if lineWidth := visibleWidthANSI(line); lineWidth < effectiveWidth {
			writeSpaces(&out, effectiveWidth-lineWidth)
		}
		out.WriteString(framePadStr)
		out.WriteByte('|')
		out.WriteByte('\n')
	}
	out.WriteString(paddingLine)
	out.WriteByte('\n')
	out.WriteString(topBottom)
	return out.String()
}

func placeCardCentered(card string, width, height int) string {
	if width <= 0 || height <= 0 {
		return card
	}
	lines := strings.Split(card, "\n")
	cardHeight := len(lines)
	cardWidth := 0
	for _, line := range lines {
		if w := visibleWidthANSI(line); w > cardWidth {
			cardWidth = w
		}
	}
	if cardWidth <= 0 {
		cardWidth = 1
	}
	if cardWidth > width {
		cardWidth = width
	}

	topPad := maxInt(0, (height-cardHeight)/2)
	leftPad := maxInt(0, (width-cardWidth)/2)
	blank := strings.Repeat(" ", width)
	leftPadStr := strings.Repeat(" ", leftPad)

	var out strings.Builder
	out.Grow((width + 1) * maxInt(height, cardHeight))

	for i := 0; i < topPad; i++ {
		out.WriteString(blank)
		out.WriteByte('\n')
	}

	renderedCardLines := 0
	for _, line := range lines {
		if renderedCardLines+topPad >= height {
			break
		}
		if visibleWidthANSI(line) > cardWidth {
			line = truncateVisible(line, cardWidth)
		}
		used := visibleWidthANSI(line)
		rightPad := maxInt(0, width-leftPad-used)
		out.WriteString(leftPadStr)
		out.WriteString(line)
		writeSpaces(&out, rightPad)
		out.WriteByte('\n')
		renderedCardLines++
	}

	for renderedCardLines+topPad < height {
		out.WriteString(blank)
		renderedCardLines++
		if renderedCardLines+topPad < height {
			out.WriteByte('\n')
		}
	}

	return out.String()
}
