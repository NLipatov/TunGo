package bubble_tea

import tuiconfig "tungo/internal/config/tui"

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

var orderedModePreferences = [...]tuiconfig.ModePreference{
	tuiconfig.ModePreferenceNone,
	tuiconfig.ModePreferenceClient,
	tuiconfig.ModePreferenceServer,
}

var orderedThemeOptions = [...]tuiconfig.Theme{
	tuiconfig.ThemeLight,
	tuiconfig.ThemeDark,
	tuiconfig.ThemeDarkHighContrast,
	tuiconfig.ThemeDarkMatrix,
	tuiconfig.ThemeDarkOcean,
	tuiconfig.ThemeDarkNord,
	tuiconfig.ThemeDarkMono,
}

func nextTheme(current tuiconfig.Theme, step int) tuiconfig.Theme {
	order := orderedThemeOptions[:]
	index := 0
	found := false
	for i, item := range order {
		if item == current {
			index = i
			found = true
			break
		}
	}
	if !found {
		index = 0
	}
	if step > 0 {
		index = (index + 1) % len(order)
	} else {
		index = (index - 1 + len(order)) % len(order)
	}
	return order[index]
}

func nextStatsUnits(current tuiconfig.StatsUnits, step int) tuiconfig.StatsUnits {
	order := []tuiconfig.StatsUnits{tuiconfig.StatsUnitsBytes, tuiconfig.StatsUnitsBiBytes}
	index := 0
	for i, item := range order {
		if item == current {
			index = i
			break
		}
	}
	if step > 0 {
		index = (index + 1) % len(order)
	} else {
		index = (index - 1 + len(order)) % len(order)
	}
	return order[index]
}

func nextModePreference(current tuiconfig.ModePreference, step int) tuiconfig.ModePreference {
	n := len(orderedModePreferences)
	idx := 0
	for i, m := range orderedModePreferences {
		if m == current {
			idx = i
			break
		}
	}
	idx = ((idx+step)%n + n) % n
	return orderedModePreferences[idx]
}

func settingsCursorUp(cursor int) int {
	if cursor > 0 {
		return cursor - 1
	}
	return 0
}

func visibleCursorToSettingsRow(cursor int, serverSupported bool) int {
	if serverSupported || cursor < settingsModeRow {
		return cursor
	}
	return cursor + 1 // skip hidden Mode row
}

func settingsVisibleRowCount(prefs tuiconfig.Configuration, serverSupported bool) int {
	if !serverSupported {
		return settingsRowsCount - 1 // Mode row hidden, AutoConnect always visible
	}
	if prefs.AutoSelectMode == tuiconfig.ModePreferenceClient {
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

func applySettingsChange(provider *Preferences, settingsCursor int, step int, serverSupported bool) tuiconfig.Configuration {
	p := provider.Current()
	switch visibleCursorToSettingsRow(settingsCursor, serverSupported) {
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
		p.AutoSelectMode = nextModePreference(p.AutoSelectMode, step)
	case settingsAutoConnectRow:
		p.AutoConnect = !p.AutoConnect
	}
	provider.update(p)
	_ = tuiconfig.Save(p)
	return p
}
