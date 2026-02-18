package bubble_tea

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpdateUIPreferences_SanitizesValues(t *testing.T) {
	t.Setenv(uiSettingsPathEnv, filepath.Join(t.TempDir(), "tui.json"))

	UpdateUIPreferences(func(p *UIPreferences) {
		p.Theme = ThemeOption("weird")
		p.Language = ""
		p.StatsUnits = StatsUnitsOption("odd")
	})
	p := CurrentUIPreferences()
	if p.Theme != ThemeLight {
		t.Fatalf("expected fallback theme light, got %q", p.Theme)
	}
	if p.Language != "en" {
		t.Fatalf("expected fallback language en, got %q", p.Language)
	}
	if p.StatsUnits != StatsUnitsBiBytes {
		t.Fatalf("expected fallback stats units bibytes, got %q", p.StatsUnits)
	}
}

func TestUIPreferences_SaveAndReloadRoundTrip(t *testing.T) {
	t.Setenv(uiSettingsPathEnv, filepath.Join(t.TempDir(), "tui.json"))

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
	t.Setenv(uiSettingsPathEnv, filepath.Join(t.TempDir(), "missing-tui.json"))

	if err := ReloadUIPreferences(); err != nil {
		t.Fatalf("reload should succeed for missing file, got: %v", err)
	}
	p := CurrentUIPreferences()
	if p.Theme != ThemeLight || p.Language != "en" || p.StatsUnits != StatsUnitsBiBytes || !p.ShowFooter {
		t.Fatalf("expected defaults for missing file, got %+v", p)
	}
}

func TestReloadUIPreferences_InvalidJSONReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.json")
	t.Setenv(uiSettingsPathEnv, path)
	if err := os.WriteFile(path, []byte("{ invalid json"), 0o644); err != nil {
		t.Fatalf("write invalid file failed: %v", err)
	}

	if err := ReloadUIPreferences(); err == nil {
		t.Fatalf("expected error for invalid json")
	}
}

func TestReloadUIPreferences_AutoThemeMigratesToLight(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.json")
	t.Setenv(uiSettingsPathEnv, path)
	payload := []byte("{\"theme\":\"auto\",\"language\":\"en\",\"stats_units\":\"bibytes\",\"show_footer\":true}\n")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("write ui file failed: %v", err)
	}

	if err := ReloadUIPreferences(); err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	p := CurrentUIPreferences()
	if p.Theme != ThemeLight {
		t.Fatalf("expected auto to migrate to light, got %q", p.Theme)
	}
}
