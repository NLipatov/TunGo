package bubble_tea

const (
	settingsThemeRow = iota
	settingsStatsUnitsRow
	settingsDataplaneStatsRow
	settingsDataplaneGraphRow
	settingsFooterRow
	settingsModeRow
	settingsAutoConnectRow
	settingsRowsCount
)

var orderedModePreferences = [...]ModePreference{
	ModePreferenceNone,
	ModePreferenceClient,
	ModePreferenceServer,
}

func nextModePreference(current ModePreference, step int) ModePreference {
	n := len(orderedModePreferences)
	idx := 0
	for i, m := range orderedModePreferences {
		if m == current {
			idx = i
			break
		}
	}
	idx = ((idx + step) % n + n) % n
	return orderedModePreferences[idx]
}

func settingsCursorUp(cursor int) int {
	if cursor > 0 {
		return cursor - 1
	}
	return 0
}

func settingsVisibleRowCount(prefs UIPreferences) int {
	if prefs.PreferredMode == ModePreferenceClient {
		return settingsRowsCount
	}
	return settingsRowsCount - 1 // auto-connect row hidden
}

func settingsCursorDown(cursor, rowCount int) int {
	if cursor < rowCount-1 {
		return cursor + 1
	}
	return rowCount - 1
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
	case settingsModeRow:
		p.PreferredMode = nextModePreference(p.PreferredMode, step)
	case settingsAutoConnectRow:
		p.AutoConnect = !p.AutoConnect
	}
	provider.update(p)
	_ = savePreferencesToDisk(p)
	return p
}
