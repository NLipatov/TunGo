package bubble_tea

import "strings"

type ModePreference string

const (
	ModePreferenceNone   ModePreference = ""
	ModePreferenceClient ModePreference = "client"
	ModePreferenceServer ModePreference = "server"
)

func isValidModePreference(m ModePreference) bool {
	return m == ModePreferenceNone || m == ModePreferenceClient || m == ModePreferenceServer
}

type ThemeOption string

const (
	ThemeLight            ThemeOption = "light"
	ThemeDark             ThemeOption = "dark"
	ThemeDarkHighContrast ThemeOption = "dark_high_contrast"
	ThemeDarkMatrix       ThemeOption = "dark_matrix"
	ThemeDarkOcean        ThemeOption = "dark_ocean"
	ThemeDarkNord         ThemeOption = "dark_nord"
	ThemeDarkMono         ThemeOption = "dark_mono"
)

type StatsUnitsOption string

const (
	StatsUnitsBytes   StatsUnitsOption = "bytes"
	StatsUnitsBiBytes StatsUnitsOption = "bibytes"
)

type UIPreferences struct {
	Theme                  ThemeOption      `json:"theme"`
	Language               string           `json:"language"`
	StatsUnits             StatsUnitsOption `json:"stats_units"`
	ShowDataplaneStats     bool             `json:"show_dataplane_stats"`
	ShowDataplaneGraph     bool             `json:"show_dataplane_graph"`
	ShowFooter             bool             `json:"show_footer"`
	AutoSelectMode         ModePreference   `json:"auto_select_mode,omitempty"`
	AutoConnect            bool             `json:"auto_connect,omitempty"`
	AutoSelectClientConfig string           `json:"auto_select_client_config,omitempty"`
}

var orderedThemeOptions = [...]ThemeOption{
	ThemeLight,
	ThemeDark,
	ThemeDarkHighContrast,
	ThemeDarkMatrix,
	ThemeDarkOcean,
	ThemeDarkNord,
	ThemeDarkMono,
}

func newUIPreferences(theme ThemeOption, language string, statsUnits StatsUnitsOption) UIPreferences {
	if !isValidTheme(theme) {
		theme = ThemeLight
	}
	if !isValidLanguage(language) {
		language = "en"
	}
	if !isValidStatsUnits(statsUnits) {
		statsUnits = StatsUnitsBiBytes
	}
	return UIPreferences{
		Theme:              theme,
		Language:           language,
		StatsUnits:         statsUnits,
		ShowDataplaneStats: true,
		ShowDataplaneGraph: true,
		ShowFooter:         true,
	}
}

func isValidTheme(theme ThemeOption) bool {
	for _, option := range orderedThemeOptions {
		if option == theme {
			return true
		}
	}
	return false
}

func isValidLanguage(language string) bool {
	return strings.TrimSpace(language) != ""
}

func isValidStatsUnits(units StatsUnitsOption) bool {
	return units == StatsUnitsBytes || units == StatsUnitsBiBytes
}
