package tui

import "strings"

type ModePreference string

const (
	ModePreferenceNone   ModePreference = ""
	ModePreferenceClient ModePreference = "client"
	ModePreferenceServer ModePreference = "server"
)

type Theme string

const (
	ThemeLight            Theme = "light"
	ThemeDark             Theme = "dark"
	ThemeDarkHighContrast Theme = "dark_high_contrast"
	ThemeDarkMatrix       Theme = "dark_matrix"
	ThemeDarkOcean        Theme = "dark_ocean"
	ThemeDarkNord         Theme = "dark_nord"
	ThemeDarkMono         Theme = "dark_mono"
)

type StatsUnits string

const (
	StatsUnitsBytes   StatsUnits = "bytes"
	StatsUnitsBiBytes StatsUnits = "bibytes"
)

type Configuration struct {
	Theme                  Theme          `json:"theme"`
	Language               string         `json:"language"`
	StatsUnits             StatsUnits     `json:"stats_units"`
	ShowDataplaneStats     bool           `json:"show_dataplane_stats"`
	ShowDataplaneGraph     bool           `json:"show_dataplane_graph"`
	ShowFooter             bool           `json:"show_footer"`
	AutoSelectMode         ModePreference `json:"auto_select_mode,omitempty"`
	AutoConnect            bool           `json:"auto_connect,omitempty"`
	AutoSelectClientConfig string         `json:"auto_select_client_config,omitempty"`
}

func Default() Configuration {
	return Configuration{
		Theme:              ThemeLight,
		Language:           "en",
		StatsUnits:         StatsUnitsBiBytes,
		ShowDataplaneStats: true,
		ShowDataplaneGraph: true,
		ShowFooter:         true,
	}
}

func normalize(configuration Configuration) Configuration {
	defaults := Default()
	if !validTheme(configuration.Theme) {
		configuration.Theme = defaults.Theme
	}
	if strings.TrimSpace(configuration.Language) == "" {
		configuration.Language = defaults.Language
	}
	if !validStatsUnits(configuration.StatsUnits) {
		configuration.StatsUnits = defaults.StatsUnits
	}
	if !validModePreference(configuration.AutoSelectMode) {
		configuration.AutoSelectMode = defaults.AutoSelectMode
	}
	return configuration
}

func validTheme(theme Theme) bool {
	switch theme {
	case ThemeLight, ThemeDark, ThemeDarkHighContrast, ThemeDarkMatrix, ThemeDarkOcean, ThemeDarkNord, ThemeDarkMono:
		return true
	default:
		return false
	}
}

func validStatsUnits(units StatsUnits) bool {
	return units == StatsUnitsBytes || units == StatsUnitsBiBytes
}

func validModePreference(mode ModePreference) bool {
	return mode == ModePreferenceNone || mode == ModePreferenceClient || mode == ModePreferenceServer
}
