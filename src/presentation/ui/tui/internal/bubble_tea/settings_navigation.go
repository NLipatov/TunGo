package bubble_tea

const (
	settingsThemeRow = iota
	settingsStatsUnitsRow
	settingsDataplaneStatsRow
	settingsDataplaneGraphRow
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

func applySettingsChange(provider *uiPreferencesProvider, settingsCursor int, step int) UIPreferences {
	p := provider.Preferences()
	switch settingsCursor {
	case settingsThemeRow:
		p.Theme = nextTheme(p.Theme, step)
	case settingsStatsUnitsRow:
		p.StatsUnits = nextStatsUnits(p.StatsUnits, step)
	case settingsDataplaneStatsRow:
		p.ShowDataplaneStats = !p.ShowDataplaneStats
	case settingsDataplaneGraphRow:
		p.ShowDataplaneGraph = !p.ShowDataplaneGraph
	case settingsFooterRow:
		p.ShowFooter = !p.ShowFooter
	}
	provider.update(p)
	_ = savePreferencesToDisk(p)
	return p
}
