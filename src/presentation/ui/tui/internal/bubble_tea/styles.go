package bubble_tea

import (
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"tungo/infrastructure/telemetry/trafficstats"
)

const (
	minCardWidth    = 36
	maxCardWidth    = 96
	sideInsetCols   = 6
	minCardHeight   = 16
	maxCardHeight   = 30
	topBottomInsets = 4
	statsValueWidth = 12
	framePadX       = 2
	framePadY       = 1
	framePadStr     = "  " // must match framePadX
	frameBorderX    = 2
	frameBorderY    = 2
	frameHorizSize  = frameBorderX + framePadX*2 // 6
	frameVertSize   = frameBorderY + framePadY*2 // 4
)

type uiStyles struct {
	headerBar   ansiTextStyle
	brand       ansiTextStyle
	headerTitle ansiTextStyle
	headerRule  ansiTextStyle
	title       ansiTextStyle
	subtitle    ansiTextStyle
	hint        ansiTextStyle
	option      ansiTextStyle
	active      ansiTextStyle
	inputFrame  ansiFrameStyle
	meta        ansiTextStyle
}

type ansiTextStyle struct {
	prefix string
	width  int
}

func (s ansiTextStyle) Width(width int) ansiTextStyle {
	s.width = width
	return s
}

func (s ansiTextStyle) Render(value string) string {
	if s.width > 0 {
		value = padRightVisible(value, s.width)
	}
	if s.prefix == "" {
		return value
	}
	return s.prefix + value + ansiReset
}

type ansiFrameStyle struct {
	borderPrefix string
	width        int
}

func (s ansiFrameStyle) Width(width int) ansiFrameStyle {
	s.width = width
	return s
}

func (s ansiFrameStyle) GetHorizontalFrameSize() int {
	return 4
}

func (s ansiFrameStyle) Render(content string) string {
	lines := strings.Split(content, "\n")
	innerWidth := 1
	if s.width > 0 {
		innerWidth = maxInt(1, s.width-s.GetHorizontalFrameSize())
	} else {
		for _, line := range lines {
			innerWidth = maxInt(innerWidth, visibleWidthANSI(line))
		}
	}
	topBottom := "+" + strings.Repeat("-", innerWidth+2) + "+"
	borderRow := "|" + strings.Repeat(" ", innerWidth+2) + "|"

	var out strings.Builder
	out.Grow((innerWidth + 6) * (len(lines) + 2))
	writeBorderLine(&out, s.borderPrefix, topBottom)
	out.WriteByte('\n')
	for i, line := range lines {
		if i > 0 {
			out.WriteByte('\n')
		}
		if visibleWidthANSI(line) > innerWidth {
			line = truncateVisible(line, innerWidth)
		}
		writeBorderLine(&out, s.borderPrefix, "| "+padRightVisible(line, innerWidth)+" |")
	}
	if len(lines) == 0 {
		writeBorderLine(&out, s.borderPrefix, borderRow)
	}
	out.WriteByte('\n')
	writeBorderLine(&out, s.borderPrefix, topBottom)
	return out.String()
}

func writeBorderLine(out *strings.Builder, prefix, line string) {
	if prefix == "" {
		out.WriteString(line)
		return
	}
	out.WriteString(prefix)
	out.WriteString(line)
	out.WriteString(ansiReset)
}

const ansiReset = "\x1b[0m"

var (
	wrapTextForBody      = wrapText
	baseANSIForThemeFunc = baseANSIForTheme
	uiStylesCacheMu      sync.RWMutex
	uiStylesCache        = map[uiStylesCacheKey]uiStyles{}
)

type uiStylesCacheKey struct {
	theme ThemeOption
}

const (
	ansiBold = "\x1b[1m"

	ansiFgBlack         = "\x1b[30m"
	ansiFgRed           = "\x1b[31m"
	ansiFgGreen         = "\x1b[32m"
	ansiFgYellow        = "\x1b[33m"
	ansiFgBlue          = "\x1b[34m"
	ansiFgMagenta       = "\x1b[35m"
	ansiFgCyan          = "\x1b[36m"
	ansiFgWhite         = "\x1b[37m"
	ansiFgBrightBlack   = "\x1b[90m"
	ansiFgBrightRed     = "\x1b[91m"
	ansiFgBrightGreen   = "\x1b[92m"
	ansiFgBrightYellow  = "\x1b[93m"
	ansiFgBrightBlue    = "\x1b[94m"
	ansiFgBrightMagenta = "\x1b[95m"
	ansiFgBrightCyan    = "\x1b[96m"
	ansiFgBrightWhite   = "\x1b[97m"

	ansiBgBlack         = "\x1b[40m"
	ansiBgRed           = "\x1b[41m"
	ansiBgGreen         = "\x1b[42m"
	ansiBgYellow        = "\x1b[43m"
	ansiBgBlue          = "\x1b[44m"
	ansiBgMagenta       = "\x1b[45m"
	ansiBgCyan          = "\x1b[46m"
	ansiBgWhite         = "\x1b[47m"
	ansiBgBrightBlack   = "\x1b[100m"
	ansiBgBrightRed     = "\x1b[101m"
	ansiBgBrightGreen   = "\x1b[102m"
	ansiBgBrightYellow  = "\x1b[103m"
	ansiBgBrightBlue    = "\x1b[104m"
	ansiBgBrightMagenta = "\x1b[105m"
	ansiBgBrightCyan    = "\x1b[106m"
	ansiBgBrightWhite   = "\x1b[107m"
)

type themePalette struct {
	background       string
	text             string
	muted            string
	brand            string
	accentText       string
	activeBackground string
	activeText       string
}

func paletteForTheme(theme ThemeOption) themePalette {
	switch theme {
	case ThemeDarkHighContrast:
		return themePalette{
			background:       ansiBgBlack,
			text:             ansiFgBrightWhite,
			muted:            ansiFgWhite,
			brand:            ansiFgBrightCyan,
			accentText:       ansiFgBrightYellow,
			activeBackground: ansiBgBrightYellow,
			activeText:       ansiFgBlack,
		}
	case ThemeDarkMatrix:
		return themePalette{
			background:       ansiBgBlack,
			text:             ansiFgBrightGreen,
			muted:            ansiFgGreen,
			brand:            ansiFgBrightCyan,
			accentText:       ansiFgGreen,
			activeBackground: ansiBgBrightGreen,
			activeText:       ansiFgBlack,
		}
	case ThemeDarkOcean:
		return themePalette{
			background:       ansiBgBlack,
			text:             ansiFgBrightWhite,
			muted:            ansiFgCyan,
			brand:            ansiFgBrightCyan,
			accentText:       ansiFgBlue,
			activeBackground: ansiBgBlue,
			activeText:       ansiFgBrightWhite,
		}
	case ThemeDarkNord:
		return themePalette{
			background:       ansiBgBrightBlack,
			text:             ansiFgWhite,
			muted:            ansiFgBrightBlack,
			brand:            ansiFgBrightCyan,
			accentText:       ansiFgCyan,
			activeBackground: ansiBgCyan,
			activeText:       ansiFgBlack,
		}
	case ThemeDarkMono:
		return themePalette{
			background:       ansiBgBlack,
			text:             ansiFgWhite,
			muted:            ansiFgBrightBlack,
			brand:            ansiFgBrightCyan,
			accentText:       ansiFgWhite,
			activeBackground: ansiBgWhite,
			activeText:       ansiFgBlack,
		}
	case ThemeDark:
		return themePalette{
			background:       ansiBgBlack,
			text:             ansiFgWhite,
			muted:            ansiFgBrightBlack,
			brand:            ansiFgBrightCyan,
			accentText:       ansiFgBrightBlue,
			activeBackground: ansiBgCyan,
			activeText:       ansiFgBlack,
		}
	case ThemeLight:
		fallthrough
	default:
		return themePalette{
			background:       ansiBgBrightWhite,
			text:             ansiFgBlack,
			muted:            ansiFgBrightBlack,
			brand:            ansiFgBrightCyan,
			accentText:       ansiFgBlue,
			activeBackground: ansiBgBlue,
			activeText:       ansiFgBrightWhite,
		}
	}
}

func ansiStylePrefix(fgPrefix, bgPrefix string, bold bool) string {
	if fgPrefix == "" || bgPrefix == "" {
		return ""
	}
	var out strings.Builder
	out.Grow(24)
	if bold {
		out.WriteString(ansiBold)
	}
	out.WriteString(fgPrefix)
	out.WriteString(bgPrefix)
	return out.String()
}

func resolveUIStyles(prefs UIPreferences) uiStyles {
	theme := prefs.Theme
	if !isValidTheme(theme) {
		theme = ThemeLight
	}
	key := uiStylesCacheKey{theme: theme}

	uiStylesCacheMu.RLock()
	cached, ok := uiStylesCache[key]
	uiStylesCacheMu.RUnlock()
	if ok {
		return cached
	}

	p := paletteForTheme(theme)
	textColor := p.text
	mutedColor := p.muted
	brandColor := p.brand
	accentTextColor := p.accentText
	activeBackgroundColor := p.activeBackground
	activeTextColor := p.activeText
	backgroundColor := p.background

	textPrefix := ansiStylePrefix(textColor, backgroundColor, false)
	mutedPrefix := ansiStylePrefix(mutedColor, backgroundColor, false)
	brandPrefix := ansiStylePrefix(brandColor, backgroundColor, false)
	rulePrefix := ansiStylePrefix(accentTextColor, backgroundColor, false)
	activePrefix := ansiStylePrefix(activeTextColor, activeBackgroundColor, true)

	styles := uiStyles{
		headerBar:   ansiTextStyle{prefix: textPrefix},
		brand:       ansiTextStyle{prefix: brandPrefix},
		headerTitle: ansiTextStyle{prefix: ansiStylePrefix(textColor, backgroundColor, true)},
		headerRule:  ansiTextStyle{prefix: rulePrefix},
		title:       ansiTextStyle{prefix: ansiStylePrefix(textColor, backgroundColor, true)},
		subtitle:    ansiTextStyle{prefix: mutedPrefix},
		hint:        ansiTextStyle{prefix: textPrefix},
		option:      ansiTextStyle{prefix: textPrefix},
		active:      ansiTextStyle{prefix: activePrefix},
		inputFrame:  ansiFrameStyle{borderPrefix: rulePrefix},
		meta:        ansiTextStyle{prefix: mutedPrefix},
	}

	uiStylesCacheMu.Lock()
	uiStylesCache[key] = styles
	uiStylesCacheMu.Unlock()

	return styles
}

func renderScreen(width, height int, title, subtitle string, body []string, hint string, prefs UIPreferences, styles uiStyles) string {
	return renderScreenWithBodyMode(width, height, title, subtitle, body, hint, true, prefs, styles)
}

func renderScreenRaw(width, height int, title, subtitle string, body []string, hint string, prefs UIPreferences, styles uiStyles) string {
	return renderScreenWithBodyMode(width, height, title, subtitle, body, hint, false, prefs, styles)
}

func renderScreenWithBodyMode(
	width, height int,
	title, subtitle string,
	body []string,
	hint string,
	wrapBodyLines bool,
	prefs UIPreferences,
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

func buildFooterBlock(styles uiStyles, prefs UIPreferences, contentWidth int, hint string) []string {
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

func formatStatsLines(prefs UIPreferences, snapshot trafficstats.Snapshot) []string {
	rxRate := formatRateForPrefs(prefs, snapshot.RXRate)
	txRate := formatRateForPrefs(prefs, snapshot.TXRate)
	rxTotal := formatTotalForPrefs(prefs, snapshot.RXBytesTotal)
	txTotal := formatTotalForPrefs(prefs, snapshot.TXBytesTotal)

	return []string{
		formatStatsLine("RX", rxRate, "TX", txRate),
		formatStatsLine("Total RX", rxTotal, "Total TX", txTotal),
	}
}

func formatStatsLine(labelA, valueA, labelB, valueB string) string {
	var b strings.Builder
	b.Grow(8 + 1 + statsValueWidth + 3 + 8 + 1 + statsValueWidth)
	writeRightPadded(&b, labelA, 8)
	b.WriteByte(' ')
	writeLeftPadded(&b, valueA, statsValueWidth)
	b.WriteString(" | ")
	writeRightPadded(&b, labelB, 8)
	b.WriteByte(' ')
	writeLeftPadded(&b, valueB, statsValueWidth)
	return b.String()
}

func writeRightPadded(b *strings.Builder, s string, width int) {
	b.WriteString(s)
	for i := len(s); i < width; i++ {
		b.WriteByte(' ')
	}
}

func writeLeftPadded(b *strings.Builder, s string, width int) {
	for i := len(s); i < width; i++ {
		b.WriteByte(' ')
	}
	b.WriteString(s)
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

func visibleWidthANSI(s string) int {
	const (
		ansiNormal = iota
		ansiEsc
		ansiCSI
		ansiOSC
		ansiST
	)

	width := 0
	state := ansiNormal
	for i := 0; i < len(s); {
		b := s[i]
		switch state {
		case ansiNormal:
			if b == '\x1b' {
				state = ansiEsc
				i++
				continue
			}
			_, size := utf8.DecodeRuneInString(s[i:])
			if size <= 0 {
				return width
			}
			width++
			i += size
		case ansiEsc:
			switch b {
			case '[':
				state = ansiCSI
			case ']':
				state = ansiOSC
			default:
				state = ansiNormal
			}
			i++
		case ansiCSI:
			i++
			if b >= 0x40 && b <= 0x7E {
				state = ansiNormal
			}
		case ansiOSC:
			if b == '\a' {
				state = ansiNormal
				i++
				continue
			}
			if b == '\x1b' {
				state = ansiST
				i++
				continue
			}
			i++
		case ansiST:
			if b == '\\' {
				state = ansiNormal
				i++
				continue
			}
			state = ansiOSC
		}
	}
	return width
}

const spaces64 = "                                                                "

func writeSpaces(out *strings.Builder, n int) {
	for n >= len(spaces64) {
		out.WriteString(spaces64)
		n -= len(spaces64)
	}
	if n > 0 {
		out.WriteString(spaces64[:n])
	}
}

func padRightVisible(s string, width int) string {
	current := visibleWidthANSI(s)
	if current >= width {
		return s
	}
	return s + strings.Repeat(" ", width-current)
}

func truncateVisible(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if visibleWidthANSI(s) <= width {
		return s
	}
	plain := stripANSI(s)
	return truncateWithEllipsis(plain, width)
}

func stripANSI(s string) string {
	const (
		ansiNormal = iota
		ansiEsc
		ansiCSI
		ansiOSC
		ansiST
	)

	var out strings.Builder
	out.Grow(len(s))
	state := ansiNormal
	for i := 0; i < len(s); {
		b := s[i]
		switch state {
		case ansiNormal:
			if b == '\x1b' {
				state = ansiEsc
				i++
				continue
			}
			_, size := utf8.DecodeRuneInString(s[i:])
			if size <= 0 {
				return out.String()
			}
			out.WriteString(s[i : i+size])
			i += size
		case ansiEsc:
			switch b {
			case '[':
				state = ansiCSI
			case ']':
				state = ansiOSC
			default:
				state = ansiNormal
			}
			i++
		case ansiCSI:
			i++
			if b >= 0x40 && b <= 0x7E {
				state = ansiNormal
			}
		case ansiOSC:
			if b == '\a' {
				state = ansiNormal
				i++
				continue
			}
			if b == '\x1b' {
				state = ansiST
				i++
				continue
			}
			i++
		case ansiST:
			if b == '\\' {
				state = ansiNormal
				i++
				continue
			}
			state = ansiOSC
		}
	}
	return out.String()
}

func enforceBaseThemeFill(s string, prefs UIPreferences) string {
	bg, fg, ok := baseANSIForThemeFunc(prefs)
	if !ok {
		return s
	}
	base := bg + fg
	lineCount := 1 + strings.Count(s, "\n")
	var out strings.Builder
	out.Grow(len(s) + lineCount*(len(base)+len(ansiReset)))

	start := 0
	for i := 0; i <= len(s); i++ {
		if i != len(s) && s[i] != '\n' {
			continue
		}
		line := s[start:i]
		out.WriteString(base)
		writeWithBaseReapplied(&out, line, base)
		out.WriteString(ansiReset)
		if i != len(s) {
			out.WriteByte('\n')
		}
		start = i + 1
	}
	return out.String()
}

func writeWithBaseReapplied(out *strings.Builder, s string, base string) {
	for i := 0; i < len(s); {
		if s[i] != '\x1b' || i+1 >= len(s) || s[i+1] != '[' {
			out.WriteByte(s[i])
			i++
			continue
		}

		seqStart := i
		i += 2
		paramsStart := i
		for i < len(s) {
			c := s[i]
			i++
			if c < '@' || c > '~' {
				continue
			}
			out.WriteString(s[seqStart:i])
			if c == 'm' && shouldReapplyBaseAfterSGR(s[paramsStart:i-1]) {
				out.WriteString(base)
			}
			break
		}
	}
}

func shouldReapplyBaseAfterSGR(params string) bool {
	if params == "" {
		return true
	}
	start := 0
	for i := 0; i <= len(params); i++ {
		if i != len(params) && params[i] != ';' {
			continue
		}
		token := strings.TrimSpace(params[start:i])
		switch token {
		case "", "0", "39", "49":
			return true
		}
		start = i + 1
	}
	return false
}

func baseANSIForTheme(prefs UIPreferences) (bg string, fg string, ok bool) {
	theme := prefs.Theme
	if !isValidTheme(theme) {
		theme = ThemeLight
	}
	p := paletteForTheme(theme)
	if p.background == "" || p.text == "" {
		return "", "", false
	}
	return p.background, p.text, true
}
