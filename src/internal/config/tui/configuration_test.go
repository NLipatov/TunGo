package tui

import "testing"

func TestDefault(t *testing.T) {
	configuration := Default()
	if configuration.Theme != ThemeLight ||
		configuration.Language != "en" ||
		configuration.StatsUnits != StatsUnitsBiBytes ||
		!configuration.ShowDataplaneStats ||
		!configuration.ShowDataplaneGraph ||
		!configuration.ShowFooter {
		t.Fatalf("Default() = %+v", configuration)
	}
}

func TestNormalizeUnsupportedValues(t *testing.T) {
	configuration := normalize(Configuration{
		Theme:          Theme("unsupported"),
		Language:       " ",
		StatsUnits:     StatsUnits("unsupported"),
		AutoSelectMode: ModePreference("unsupported"),
	})
	defaults := Default()
	if configuration.Theme != defaults.Theme ||
		configuration.Language != defaults.Language ||
		configuration.StatsUnits != defaults.StatsUnits ||
		configuration.AutoSelectMode != defaults.AutoSelectMode {
		t.Fatalf("normalize() = %+v", configuration)
	}
}
