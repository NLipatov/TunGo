package bubble_tea

const (
	settingsThemeRow = iota
	settingsStatsUnitsRow
	settingsFooterRow
	settingsRowsCount
)

func settingsCursorUp(cursor int) int {
	if cursor > 0 {
		return cursor - 1
	}
	return 0
}

func settingsCursorDown(cursor int) int {
	if cursor < settingsRowsCount-1 {
		return cursor + 1
	}
	return settingsRowsCount - 1
}

func applySettingsChange(settingsCursor int, step int) UIPreferences {
	return UpdateUIPreferences(func(p *UIPreferences) {
		switch settingsCursor {
		case settingsThemeRow:
			p.Theme = nextTheme(p.Theme, step)
		case settingsStatsUnitsRow:
			p.StatsUnits = nextStatsUnits(p.StatsUnits, step)
		case settingsFooterRow:
			p.ShowFooter = !p.ShowFooter
		}
	})
}
