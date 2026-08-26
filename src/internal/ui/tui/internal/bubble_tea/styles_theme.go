package bubble_tea

import (
	"strings"
	tuiconfig "tungo/internal/config/tui"
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

type themePalette struct {
	background       string
	text             string
	muted            string
	brand            string
	accentText       string
	activeBackground string
	activeText       string
}

func paletteForTheme(theme tuiconfig.Theme) themePalette {
	switch theme {
	case tuiconfig.ThemeDarkHighContrast:
		return themePalette{
			background:       ansiBgBlack,
			text:             ansiFgBrightWhite,
			muted:            ansiFgWhite,
			brand:            ansiFgBrightCyan,
			accentText:       ansiFgBrightYellow,
			activeBackground: ansiBgBrightYellow,
			activeText:       ansiFgBlack,
		}
	case tuiconfig.ThemeDarkMatrix:
		return themePalette{
			background:       ansiBgBlack,
			text:             ansiFgBrightGreen,
			muted:            ansiFgGreen,
			brand:            ansiFgBrightCyan,
			accentText:       ansiFgGreen,
			activeBackground: ansiBgBrightGreen,
			activeText:       ansiFgBlack,
		}
	case tuiconfig.ThemeDarkOcean:
		return themePalette{
			background:       ansiBgBlack,
			text:             ansiFgBrightWhite,
			muted:            ansiFgCyan,
			brand:            ansiFgBrightCyan,
			accentText:       ansiFgBlue,
			activeBackground: ansiBgBlue,
			activeText:       ansiFgBrightWhite,
		}
	case tuiconfig.ThemeDarkNord:
		return themePalette{
			background:       ansiBgBrightBlack,
			text:             ansiFgWhite,
			muted:            ansiFgBrightBlack,
			brand:            ansiFgBrightCyan,
			accentText:       ansiFgCyan,
			activeBackground: ansiBgCyan,
			activeText:       ansiFgBlack,
		}
	case tuiconfig.ThemeDarkMono:
		return themePalette{
			background:       ansiBgBlack,
			text:             ansiFgWhite,
			muted:            ansiFgBrightBlack,
			brand:            ansiFgBrightCyan,
			accentText:       ansiFgWhite,
			activeBackground: ansiBgWhite,
			activeText:       ansiFgBlack,
		}
	case tuiconfig.ThemeDark:
		return themePalette{
			background:       ansiBgBlack,
			text:             ansiFgWhite,
			muted:            ansiFgBrightBlack,
			brand:            ansiFgBrightCyan,
			accentText:       ansiFgBrightBlue,
			activeBackground: ansiBgCyan,
			activeText:       ansiFgBlack,
		}
	case tuiconfig.ThemeLight:
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

func resolveUIStyles(prefs tuiconfig.Configuration) uiStyles {
	p := paletteForTheme(prefs.Theme)
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

	return uiStyles{
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
}
