package bubble_tea

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadTestSettings(t *testing.T, path string) *uiPreferencesProvider {
	t.Helper()
	loaded, err := loadPreferences(defaultPrefsStorage{filePath: path})
	if err != nil {
		return newDefaultUIPreferencesProvider()
	}
	return newUIPreferencesProvider(loaded)
}

func TestNewUIPreferences_SanitizesValues(t *testing.T) {
	p := newUIPreferences(ThemeOption("weird"), "", StatsUnitsOption("odd"))
	if p.Theme != ThemeLight {
		t.Fatalf("expected fallback theme light, got %q", p.Theme)
	}
	if p.Language != "en" {
		t.Fatalf("expected fallback language en, got %q", p.Language)
	}
	if p.StatsUnits != StatsUnitsBiBytes {
		t.Fatalf("expected fallback stats units bibytes, got %q", p.StatsUnits)
	}
	if !p.ShowDataplaneStats || !p.ShowDataplaneGraph || !p.ShowFooter {
		t.Fatalf("expected booleans to default on, got %+v", p)
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

func TestUISettings_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.json")
	st := defaultPrefsStorage{filePath: path}
	p := UIPreferences{
		Theme: ThemeDark, Language: "en", StatsUnits: StatsUnitsBytes,
		ShowDataplaneStats: false, ShowDataplaneGraph: false, ShowFooter: false,
	}
	if err := savePreferencesTo(st, p); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	s := loadTestSettings(t, path)
	loaded := s.Preferences()
	if loaded.Theme != ThemeDark ||
		loaded.ShowFooter ||
		loaded.StatsUnits != StatsUnitsBytes ||
		loaded.ShowDataplaneStats ||
		loaded.ShowDataplaneGraph {
		t.Fatalf("expected reloaded settings from disk, got %+v", loaded)
	}
}

func TestLoadPreferences_MissingFileUsesDefaults(t *testing.T) {
	s := loadTestSettings(t, filepath.Join(t.TempDir(), "missing-tui.json"))
	p := s.Preferences()
	if p.Theme != ThemeLight ||
		p.Language != "en" ||
		p.StatsUnits != StatsUnitsBiBytes ||
		!p.ShowDataplaneStats ||
		!p.ShowDataplaneGraph ||
		!p.ShowFooter {
		t.Fatalf("expected defaults for missing file, got %+v", p)
	}
}

func TestLoadPreferences_InvalidJSONFallsBackToDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.json")
	if err := os.WriteFile(path, []byte("{ invalid json"), 0o644); err != nil {
		t.Fatalf("write invalid file failed: %v", err)
	}
	s := loadTestSettings(t, path)
	if s.Preferences() != newDefaultUIPreferencesProvider().Preferences() {
		t.Fatalf("expected defaults after load error, got %+v", s.Preferences())
	}
}

func TestLoadPreferences_UnknownThemeFallsBackToLight(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.json")
	payload := []byte("{\"theme\":\"nonexistent\",\"language\":\"en\",\"stats_units\":\"bibytes\",\"show_footer\":true}\n")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("write ui file failed: %v", err)
	}
	s := loadTestSettings(t, path)
	if s.Preferences().Theme != ThemeLight {
		t.Fatalf("expected unknown theme to fall back to light, got %q", s.Preferences().Theme)
	}
}

func TestLoadPreferences_LoadsSuccessfully(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.json")
	if err := os.WriteFile(path, []byte("{\"theme\":\"dark\",\"language\":\"en\",\"stats_units\":\"bytes\",\"show_footer\":false}\n"), 0o644); err != nil {
		t.Fatalf("write ui file failed: %v", err)
	}
	s := loadTestSettings(t, path)
	p := s.Preferences()
	if p.Theme != ThemeDark ||
		p.StatsUnits != StatsUnitsBytes ||
		!p.ShowDataplaneStats ||
		!p.ShowDataplaneGraph ||
		p.ShowFooter {
		t.Fatalf("unexpected loaded preferences: %+v", p)
	}
}

func TestLoadPreferences_MissingDataplaneKeysDefaultsToEnabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.json")
	payload := []byte("{\"theme\":\"dark\",\"language\":\"en\",\"stats_units\":\"bytes\",\"show_footer\":true}\n")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("write ui file failed: %v", err)
	}
	s := loadTestSettings(t, path)
	p := s.Preferences()
	if !p.ShowDataplaneStats || !p.ShowDataplaneGraph {
		t.Fatalf("expected missing dataplane flags to default true, got %+v", p)
	}
}

func TestSettings_NonEmpty(t *testing.T) {
	s := newDefaultUIPreferencesProvider()
	p := s.Preferences()
	if p.Language == "" || !p.ShowDataplaneStats || !p.ShowDataplaneGraph {
		t.Fatalf("expected initialized preferences, got %+v", p)
	}
}

func TestDefaultPrefsStorage_ReadError(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "tui.json"), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	st := defaultPrefsStorage{filePath: filepath.Join(dir, "tui.json")}
	if _, err := st.Read(); err == nil {
		t.Fatal("expected read error when tui.json is a directory")
	}
}

func TestDefaultPrefsStorage_MkdirError(t *testing.T) {
	parentFile := filepath.Join(t.TempDir(), "file-parent")
	if err := os.WriteFile(parentFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	st := defaultPrefsStorage{filePath: filepath.Join(parentFile, "tui.json")}
	if err := st.Write([]byte("{}")); err == nil {
		t.Fatal("expected mkdir error when parent path is file")
	}
}

func TestDefaultPrefsStorage_WriteTempError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.json")
	if err := os.MkdirAll(path+".tmp", 0o755); err != nil {
		t.Fatalf("mkdir tmp dir failed: %v", err)
	}
	st := defaultPrefsStorage{filePath: path}
	if err := st.Write([]byte("{}")); err == nil {
		t.Fatal("expected write error when tmp path is directory")
	}
}

func TestDefaultPrefsStorage_RenameError(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "tui.json"), 0o755); err != nil {
		t.Fatalf("mkdir target dir failed: %v", err)
	}
	st := defaultPrefsStorage{filePath: filepath.Join(dir, "tui.json")}
	if err := st.Write([]byte("{}")); err == nil {
		t.Fatal("expected rename error when destination is directory")
	}
}

// ---------------------------------------------------------------------------
// isValidModePreference
// ---------------------------------------------------------------------------

func TestIsValidModePreference_ValidValues(t *testing.T) {
	for _, v := range []ModePreference{ModePreferenceNone, ModePreferenceClient, ModePreferenceServer} {
		if !isValidModePreference(v) {
			t.Errorf("expected %q to be valid", v)
		}
	}
}

func TestIsValidModePreference_InvalidValues(t *testing.T) {
	for _, v := range []ModePreference{"admin", "superuser", "root", "0"} {
		if isValidModePreference(v) {
			t.Errorf("expected %q to be invalid", v)
		}
	}
}

// ---------------------------------------------------------------------------
// Loading of new UIPreferences fields
// ---------------------------------------------------------------------------

func TestLoadPreferences_AutoSelectMode_Client_Loaded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.json")
	if err := os.WriteFile(path, []byte(`{"auto_select_mode":"client"}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	s := loadTestSettings(t, path)
	if s.Preferences().AutoSelectMode != ModePreferenceClient {
		t.Fatalf("expected ModePreferenceClient, got %q", s.Preferences().AutoSelectMode)
	}
}

func TestLoadPreferences_AutoSelectMode_Server_Loaded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.json")
	if err := os.WriteFile(path, []byte(`{"auto_select_mode":"server"}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	s := loadTestSettings(t, path)
	if s.Preferences().AutoSelectMode != ModePreferenceServer {
		t.Fatalf("expected ModePreferenceServer, got %q", s.Preferences().AutoSelectMode)
	}
}

func TestLoadPreferences_AutoSelectMode_InvalidValue_Ignored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.json")
	if err := os.WriteFile(path, []byte(`{"auto_select_mode":"superuser"}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	s := loadTestSettings(t, path)
	if s.Preferences().AutoSelectMode != ModePreferenceNone {
		t.Fatalf("expected ModePreferenceNone for invalid value, got %q", s.Preferences().AutoSelectMode)
	}
}

func TestLoadPreferences_AutoSelectMode_BackwardCompat_OldKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.json")
	if err := os.WriteFile(path, []byte(`{"preferred_mode":"client"}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	s := loadTestSettings(t, path)
	if s.Preferences().AutoSelectMode != ModePreferenceClient {
		t.Fatalf("expected backward-compat load from preferred_mode, got %q", s.Preferences().AutoSelectMode)
	}
}

func TestLoadPreferences_AutoConnect_True_Loaded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.json")
	if err := os.WriteFile(path, []byte(`{"auto_connect":true}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	s := loadTestSettings(t, path)
	if !s.Preferences().AutoConnect {
		t.Fatal("expected AutoConnect=true")
	}
}

func TestLoadPreferences_AutoConnect_MissingKey_DefaultsFalse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.json")
	if err := os.WriteFile(path, []byte(`{"theme":"dark"}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	s := loadTestSettings(t, path)
	if s.Preferences().AutoConnect {
		t.Fatal("expected AutoConnect=false when key absent")
	}
}

func TestLoadPreferences_AutoSelectClientConfig_Loaded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.json")
	if err := os.WriteFile(path, []byte(`{"auto_select_client_config":"/etc/tungo/client.json"}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	s := loadTestSettings(t, path)
	if s.Preferences().AutoSelectClientConfig != "/etc/tungo/client.json" {
		t.Fatalf("expected /etc/tungo/client.json, got %q", s.Preferences().AutoSelectClientConfig)
	}
}

func TestLoadPreferences_AutoSelectClientConfig_BackwardCompat_OldKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.json")
	if err := os.WriteFile(path, []byte(`{"last_client_config":"/etc/tungo/client.json"}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	s := loadTestSettings(t, path)
	if s.Preferences().AutoSelectClientConfig != "/etc/tungo/client.json" {
		t.Fatalf("expected backward-compat load from last_client_config, got %q", s.Preferences().AutoSelectClientConfig)
	}
}

func TestUISettings_RoundTrip_NewFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.json")
	st := defaultPrefsStorage{filePath: path}
	p := UIPreferences{
		Theme:                  ThemeDark,
		Language:               "en",
		StatsUnits:             StatsUnitsBytes,
		ShowDataplaneStats:     true,
		ShowDataplaneGraph:     true,
		ShowFooter:             true,
		AutoSelectMode:         ModePreferenceClient,
		AutoConnect:            true,
		AutoSelectClientConfig: "/some/path/cfg.json",
	}
	if err := savePreferencesTo(st, p); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded := loadTestSettings(t, path).Preferences()
	if loaded.AutoSelectMode != ModePreferenceClient {
		t.Errorf("AutoSelectMode: got %q, want client", loaded.AutoSelectMode)
	}
	if !loaded.AutoConnect {
		t.Error("AutoConnect: expected true")
	}
	if loaded.AutoSelectClientConfig != "/some/path/cfg.json" {
		t.Errorf("AutoSelectClientConfig: got %q, want /some/path/cfg.json", loaded.AutoSelectClientConfig)
	}
}
