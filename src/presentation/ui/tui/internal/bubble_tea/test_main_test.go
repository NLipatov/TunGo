package bubble_tea

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Disable disk persistence during tests to avoid modifying the user's
	// real tui.json config file. Tests that explicitly test persistence
	// use t.Setenv(uiSettingsPathEnv, ...) to redirect to a temp dir.
	persistPrefsFunc = func(UIPreferences) error { return nil }
	os.Exit(m.Run())
}
