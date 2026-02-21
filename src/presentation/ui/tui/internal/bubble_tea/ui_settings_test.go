package bubble_tea

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
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
	if !p.ShowDataplaneStats || !p.ShowDataplaneGraph {
		t.Fatalf("expected dataplane stats/graph to default on, got %+v", p)
	}
}

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

func TestUIPreferences_SaveAndReloadRoundTrip(t *testing.T) {
	t.Setenv(uiSettingsPathEnv, filepath.Join(t.TempDir(), "tui.json"))

	UpdateUIPreferences(func(p *UIPreferences) {
		p.Theme = ThemeDark
		p.Language = "en"
		p.StatsUnits = StatsUnitsBytes
		p.ShowDataplaneStats = false
		p.ShowDataplaneGraph = false
		p.ShowFooter = false
	})
	if err := SaveUIPreferences(); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	preferences.Store(UIPreferences{
		Theme: ThemeLight, Language: "en", StatsUnits: StatsUnitsBiBytes,
		ShowDataplaneStats: true, ShowDataplaneGraph: true, ShowFooter: true,
	})

	if err := ReloadUIPreferences(); err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	p := CurrentUIPreferences()
	if p.Theme != ThemeDark ||
		p.ShowFooter ||
		p.StatsUnits != StatsUnitsBytes ||
		p.ShowDataplaneStats ||
		p.ShowDataplaneGraph {
		t.Fatalf("expected reloaded settings from disk, got %+v", p)
	}
}

func TestReloadUIPreferences_MissingFileUsesDefaults(t *testing.T) {
	t.Setenv(uiSettingsPathEnv, filepath.Join(t.TempDir(), "missing-tui.json"))

	if err := ReloadUIPreferences(); err != nil {
		t.Fatalf("reload should succeed for missing file, got: %v", err)
	}
	p := CurrentUIPreferences()
	if p.Theme != ThemeLight ||
		p.Language != "en" ||
		p.StatsUnits != StatsUnitsBiBytes ||
		!p.ShowDataplaneStats ||
		!p.ShowDataplaneGraph ||
		!p.ShowFooter {
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

func TestReloadUIPreferences_LegacyThemeMigratesToSupportedTheme(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.json")
	t.Setenv(uiSettingsPathEnv, path)
	payload := []byte("{\"theme\":\"light_arctic\",\"language\":\"en\",\"stats_units\":\"bibytes\",\"show_footer\":true}\n")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("write ui file failed: %v", err)
	}

	if err := ReloadUIPreferences(); err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	p := CurrentUIPreferences()
	if p.Theme != ThemeLight {
		t.Fatalf("expected legacy light_arctic to migrate to light, got %q", p.Theme)
	}
}

func TestCurrentUIPreferences_NonEmptyAfterInit(t *testing.T) {
	p := CurrentUIPreferences()
	if p.Language == "" || !p.ShowDataplaneStats || !p.ShowDataplaneGraph {
		t.Fatalf("expected initialized preferences, got %+v", p)
	}
}

func TestCurrentUIPreferences_FallbacksForAtomicEdgeCases(t *testing.T) {
	original := CurrentUIPreferences()
	t.Cleanup(func() {
		preferences = atomic.Value{}
		preferences.Store(original)
	})

	preferences = atomic.Value{}
	p := CurrentUIPreferences()
	if p != defaultUIPreferences() {
		t.Fatalf("expected defaults when atomic value is empty, got %+v", p)
	}

	preferences.Store("unexpected")
	p = CurrentUIPreferences()
	if p != defaultUIPreferences() {
		t.Fatalf("expected defaults for unexpected atomic type, got %+v", p)
	}
}

func TestUIPreferencesPath_UsesEnvOverride(t *testing.T) {
	customPath := filepath.Join(t.TempDir(), "custom", "ui.json")
	t.Setenv(uiSettingsPathEnv, customPath)
	if got := uiPreferencesPath(); got != customPath {
		t.Fatalf("expected custom path %q, got %q", customPath, got)
	}
}

func TestDefaultUIPreferencesPath_UnixEtcTungo(t *testing.T) {
	if path := defaultUIPreferencesPath(); runtime.GOOS != "windows" &&
		!strings.Contains(path, string(filepath.Separator)+"etc"+string(filepath.Separator)+"tungo"+string(filepath.Separator)+"tui.json") {
		t.Fatalf("expected unix default path under /etc/tungo, got %q", path)
	}
}

func TestDefaultUIPreferencesPath_Windows(t *testing.T) {
	prevGOOS := runtimeGOOS
	t.Cleanup(func() { runtimeGOOS = prevGOOS })

	runtimeGOOS = "windows"
	t.Setenv("ProgramData", `C:\ProgramData`)
	if got := defaultUIPreferencesPath(); got != filepath.Join(`C:\ProgramData`, "TunGo", "tui.json") {
		t.Fatalf("unexpected windows default path: %q", got)
	}
}

func TestDefaultUIPreferencesPath_WindowsFallbackProgramData(t *testing.T) {
	prevGOOS := runtimeGOOS
	t.Cleanup(func() { runtimeGOOS = prevGOOS })

	runtimeGOOS = "windows"
	t.Setenv("ProgramData", "")
	if got := defaultUIPreferencesPath(); got != filepath.Join(`C:\ProgramData`, "TunGo", "tui.json") {
		t.Fatalf("unexpected windows fallback path: %q", got)
	}
}

func TestLoadUIPreferencesFromDisk_ReadError(t *testing.T) {
	dirPath := filepath.Join(t.TempDir(), "is-a-dir")
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	t.Setenv(uiSettingsPathEnv, dirPath)
	if _, err := loadUIPreferencesFromDisk(); err == nil {
		t.Fatal("expected read error when ui path points to a directory")
	}
}

func TestPersistUIPreferencesToDisk_MkdirError(t *testing.T) {
	parentFile := filepath.Join(t.TempDir(), "file-parent")
	if err := os.WriteFile(parentFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	t.Setenv(uiSettingsPathEnv, filepath.Join(parentFile, "tui.json"))
	if err := persistUIPreferencesToDisk(defaultUIPreferences()); err == nil {
		t.Fatal("expected mkdir error when parent path is file")
	}
}

func TestPersistUIPreferencesToDisk_WriteTempError(t *testing.T) {
	baseDir := t.TempDir()
	path := filepath.Join(baseDir, "tui.json")
	t.Setenv(uiSettingsPathEnv, path)

	if err := os.MkdirAll(path+".tmp", 0o755); err != nil {
		t.Fatalf("mkdir tmp dir failed: %v", err)
	}

	if err := persistUIPreferencesToDisk(defaultUIPreferences()); err == nil {
		t.Fatal("expected write error when tmp path is directory")
	}
}

func TestPersistUIPreferencesToDisk_RenameError(t *testing.T) {
	baseDir := t.TempDir()
	path := filepath.Join(baseDir, "target-dir")
	t.Setenv(uiSettingsPathEnv, path)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir target dir failed: %v", err)
	}

	if err := persistUIPreferencesToDisk(defaultUIPreferences()); err == nil {
		t.Fatal("expected rename error when destination is directory")
	}
}

func TestPersistUIPreferencesToDisk_MarshalError(t *testing.T) {
	prevMarshal := marshalUIPreferences
	t.Cleanup(func() { marshalUIPreferences = prevMarshal })
	marshalUIPreferences = func(UIPreferences) ([]byte, error) {
		return nil, errors.New("marshal failed")
	}

	t.Setenv(uiSettingsPathEnv, filepath.Join(t.TempDir(), "tui.json"))
	if err := persistUIPreferencesToDisk(defaultUIPreferences()); err == nil || err.Error() != "marshal failed" {
		t.Fatalf("expected marshal failed, got %v", err)
	}
}

func TestInitializeUIPreferences_LoadErrorFallsBackToDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.json")
	t.Setenv(uiSettingsPathEnv, path)
	if err := os.WriteFile(path, []byte("{invalid"), 0o644); err != nil {
		t.Fatalf("write invalid file failed: %v", err)
	}

	initializeUIPreferences()
	p := CurrentUIPreferences()
	if p != defaultUIPreferences() {
		t.Fatalf("expected defaults after load error, got %+v", p)
	}
}

func TestInitializeUIPreferences_LoadSuccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.json")
	t.Setenv(uiSettingsPathEnv, path)
	if err := os.WriteFile(path, []byte("{\"theme\":\"dark\",\"language\":\"en\",\"stats_units\":\"bytes\",\"show_footer\":false}\n"), 0o644); err != nil {
		t.Fatalf("write ui file failed: %v", err)
	}

	initializeUIPreferences()
	p := CurrentUIPreferences()
	if p.Theme != ThemeDark ||
		p.StatsUnits != StatsUnitsBytes ||
		!p.ShowDataplaneStats ||
		!p.ShowDataplaneGraph ||
		p.ShowFooter {
		t.Fatalf("unexpected loaded preferences: %+v", p)
	}
}

func TestReloadUIPreferences_MissingDataplaneKeys_DefaultsToEnabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.json")
	t.Setenv(uiSettingsPathEnv, path)
	payload := []byte("{\"theme\":\"dark\",\"language\":\"en\",\"stats_units\":\"bytes\",\"show_footer\":true}\n")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("write ui file failed: %v", err)
	}

	if err := ReloadUIPreferences(); err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	p := CurrentUIPreferences()
	if !p.ShowDataplaneStats || !p.ShowDataplaneGraph {
		t.Fatalf("expected missing dataplane flags to default true, got %+v", p)
	}
}
