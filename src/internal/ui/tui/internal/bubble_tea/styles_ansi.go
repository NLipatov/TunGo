package bubble_tea

import (
	"strings"
	"unicode/utf8"
)

const ansiReset = "\x1b[0m"

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

	spaces64 = "                                                                "
)

type ansiTextStyle struct {
	prefix string
	width  int
}

type ansiFrameStyle struct {
	borderPrefix string
	width        int
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
	bg, fg := baseANSIForTheme(prefs)
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

func baseANSIForTheme(prefs UIPreferences) (bg string, fg string) {
	theme := prefs.Theme
	if !isValidTheme(theme) {
		theme = ThemeLight
	}
	p := paletteForTheme(theme)
	return p.background, p.text
}
