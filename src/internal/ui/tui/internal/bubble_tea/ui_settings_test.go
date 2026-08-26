package bubble_tea

import (
	"strings"
	"testing"

	tuiconfig "tungo/internal/config/tui"
)

func TestOrderedThemeOptions_HasOneLightAndSixDarkThemes(t *testing.T) {
	if len(orderedThemeOptions) != 7 {
		t.Fatalf("expected 7 themes, got %d", len(orderedThemeOptions))
	}
	lightCount := 0
	darkCount := 0
	for _, theme := range orderedThemeOptions {
		switch {
		case strings.HasPrefix(string(theme), "light"):
			lightCount++
		case strings.HasPrefix(string(theme), "dark"):
			darkCount++
		}
	}
	if lightCount != 1 || darkCount != 6 {
		t.Fatalf("expected 1 light and 6 dark themes, got light=%d dark=%d", lightCount, darkCount)
	}
}

func TestNewDefaultPreferences(t *testing.T) {
	if got := newDefaultPreferences().Current(); got != tuiconfig.Default() {
		t.Fatalf("newDefaultPreferences() = %+v, want %+v", got, tuiconfig.Default())
	}
}

func TestPreferences_DisableAutoConnect_NoOpWhenAlreadyDisabled(t *testing.T) {
	preferences := newPreferences(tuiconfig.Configuration{AutoConnect: false})

	if err := preferences.DisableAutoConnect(); err != nil {
		t.Fatalf("DisableAutoConnect() error = %v", err)
	}
	if preferences.Current().AutoConnect {
		t.Fatal("expected AutoConnect to remain disabled")
	}
}
