package bubble_tea

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpdateUIPreferences_SanitizesValues(t *testing.T) {
	t.Setenv(uiSettingsPathEnv, filepath.Join(t.TempDir(), "ui.json"))

	UpdateUIPreferences(func(p *UIPreferences) {
		p.Theme = ThemeOption("weird")
		p.Language = ""
		p.StatsUnits = StatsUnitsOption("odd")
	})
	p := CurrentUIPreferences()
	if p.Theme != ThemeAuto {
		t.Fatalf("expected fallback theme auto, got %q", p.Theme)
	}
	if p.Language != "en" {
		t.Fatalf("expected fallback language en, got %q", p.Language)
	}
	if p.StatsUnits != StatsUnitsBiBytes {
		t.Fatalf("expected fallback stats units bibytes, got %q", p.StatsUnits)
	}
}

func TestUIPreferences_SaveAndReloadRoundTrip(t *testing.T) {
	t.Setenv(uiSettingsPathEnv, filepath.Join(t.TempDir(), "ui.json"))

	UpdateUIPreferences(func(p *UIPreferences) {
		p.Theme = ThemeDark
		p.Language = "en"
		p.StatsUnits = StatsUnitsBytes
		p.ShowFooter = false
	})
	if err := SaveUIPreferences(); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	preferences.Store(UIPreferences{Theme: ThemeLight, Language: "en", StatsUnits: StatsUnitsBiBytes, ShowFooter: true})

	if err := ReloadUIPreferences(); err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	p := CurrentUIPreferences()
	if p.Theme != ThemeDark || p.ShowFooter || p.StatsUnits != StatsUnitsBytes {
		t.Fatalf("expected reloaded settings from disk, got %+v", p)
	}
}

func TestReloadUIPreferences_MissingFileUsesDefaults(t *testing.T) {
	t.Setenv(uiSettingsPathEnv, filepath.Join(t.TempDir(), "missing-ui.json"))

	if err := ReloadUIPreferences(); err != nil {
		t.Fatalf("reload should succeed for missing file, got: %v", err)
	}
	p := CurrentUIPreferences()
	if p.Theme != ThemeAuto || p.Language != "en" || p.StatsUnits != StatsUnitsBiBytes || !p.ShowFooter {
		t.Fatalf("expected defaults for missing file, got %+v", p)
	}
}

func TestReloadUIPreferences_InvalidJSONReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ui.json")
	t.Setenv(uiSettingsPathEnv, path)
	if err := os.WriteFile(path, []byte("{ invalid json"), 0o644); err != nil {
		t.Fatalf("write invalid file failed: %v", err)
	}

	if err := ReloadUIPreferences(); err == nil {
		t.Fatalf("expected error for invalid json")
	}
}
