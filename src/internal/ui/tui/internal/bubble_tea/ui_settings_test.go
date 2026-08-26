package bubble_tea

import (
	"errors"
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
	preferences := testPreferences(tuiconfig.Configuration{AutoConnect: false})

	if err := preferences.DisableAutoConnect(); err != nil {
		t.Fatalf("DisableAutoConnect() error = %v", err)
	}
	if preferences.Current().AutoConnect {
		t.Fatal("expected AutoConnect to remain disabled")
	}
}

func TestPreferences_DisableAutoConnectSavesChange(t *testing.T) {
	preferences := testPreferences(tuiconfig.Configuration{AutoConnect: true})
	var saved tuiconfig.Configuration
	preferences.save = func(configuration tuiconfig.Configuration) error {
		saved = configuration
		return nil
	}

	if err := preferences.DisableAutoConnect(); err != nil {
		t.Fatal(err)
	}
	if saved.AutoConnect || preferences.Current().AutoConnect {
		t.Fatal("AutoConnect was not disabled in persisted and current preferences")
	}
}

func TestPreferences_DisableAutoConnectKeepsSessionChangeOnSaveFailure(t *testing.T) {
	preferences := testPreferences(tuiconfig.Configuration{AutoConnect: true})
	wantErr := errors.New("save failed")
	preferences.save = func(tuiconfig.Configuration) error { return wantErr }

	if err := preferences.DisableAutoConnect(); !errors.Is(err, wantErr) {
		t.Fatalf("DisableAutoConnect() error = %v, want %v", err, wantErr)
	}
	if preferences.Current().AutoConnect {
		t.Fatal("AutoConnect remained enabled in the current session")
	}
}

func TestPreferences_UpdateSavesBeforeChangingCurrentValue(t *testing.T) {
	initial := tuiconfig.Configuration{Theme: tuiconfig.ThemeLight}
	updated := tuiconfig.Configuration{Theme: tuiconfig.ThemeDark}
	preferences := testPreferences(initial)
	wantErr := errors.New("save failed")
	preferences.save = func(got tuiconfig.Configuration) error {
		if got != updated {
			t.Fatalf("saved configuration = %+v, want %+v", got, updated)
		}
		if preferences.Current() != initial {
			t.Fatal("current preferences changed before save completed")
		}
		return wantErr
	}

	if err := preferences.update(updated); !errors.Is(err, wantErr) {
		t.Fatalf("update() error = %v, want %v", err, wantErr)
	}
	if preferences.Current() != updated {
		t.Fatal("failed save was not applied to the current session")
	}
}
