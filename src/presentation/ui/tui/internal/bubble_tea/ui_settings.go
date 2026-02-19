package bubble_tea

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
)

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
	Theme              ThemeOption      `json:"theme"`
	Language           string           `json:"language"`
	StatsUnits         StatsUnitsOption `json:"stats_units"`
	ShowDataplaneStats bool             `json:"show_dataplane_stats"`
	ShowDataplaneGraph bool             `json:"show_dataplane_graph"`
	ShowFooter         bool             `json:"show_footer"`
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

var legacyThemeMigration = map[ThemeOption]ThemeOption{
	ThemeOption("light_arctic"):   ThemeLight,
	ThemeOption("light_ivory"):    ThemeLight,
	ThemeOption("light_mint"):     ThemeLight,
	ThemeOption("light_sand"):     ThemeLight,
	ThemeOption("light_lavender"): ThemeLight,
	ThemeOption("light_slate"):    ThemeLight,
	ThemeOption("light_sky"):      ThemeLight,
	ThemeOption("light_peach"):    ThemeLight,
	ThemeOption("light_mono"):     ThemeLight,
	ThemeOption("light_azure"):    ThemeLight,
	ThemeOption("light_cyan"):     ThemeLight,
	ThemeOption("dark_midnight"):  ThemeDarkOcean,
	ThemeOption("dark_ember"):     ThemeDarkHighContrast,
	ThemeOption("dark_violet"):    ThemeDark,
	ThemeOption("dark_forest"):    ThemeDarkMatrix,
	ThemeOption("dark_graphite"):  ThemeDarkNord,
	ThemeOption("dark_cyber"):     ThemeDark,
}

var (
	preferences          atomic.Value
	prefsMu              sync.Mutex
	runtimeGOOS          = runtime.GOOS
	marshalUIPreferences = func(p UIPreferences) ([]byte, error) {
		return json.MarshalIndent(p, "", "  ")
	}
)

const uiSettingsPathEnv = "TUNGO_UI_SETTINGS_PATH"

func init() {
	initializeUIPreferences()
}

func initializeUIPreferences() {
	p := defaultUIPreferences()
	if loaded, err := loadUIPreferencesFromDisk(); err == nil {
		p = loaded
	}
	preferences.Store(p)
}

func CurrentUIPreferences() UIPreferences {
	v := preferences.Load()
	if v == nil {
		return defaultUIPreferences()
	}
	p, ok := v.(UIPreferences)
	if !ok {
		return defaultUIPreferences()
	}
	return p
}

func UpdateUIPreferences(update func(p *UIPreferences)) UIPreferences {
	prefsMu.Lock()
	defer prefsMu.Unlock()

	p := CurrentUIPreferences()
	if update != nil {
		update(&p)
	}
	p = sanitizeUIPreferences(p)
	preferences.Store(p)
	_ = persistUIPreferencesToDisk(p)
	return p
}

func ReloadUIPreferences() error {
	prefsMu.Lock()
	defer prefsMu.Unlock()

	p, err := loadUIPreferencesFromDisk()
	if err != nil {
		return err
	}
	preferences.Store(p)
	return nil
}

func SaveUIPreferences() error {
	prefsMu.Lock()
	defer prefsMu.Unlock()

	return persistUIPreferencesToDisk(CurrentUIPreferences())
}

func defaultUIPreferences() UIPreferences {
	return UIPreferences{
		Theme:              ThemeLight,
		Language:           "en",
		StatsUnits:         StatsUnitsBiBytes,
		ShowDataplaneStats: true,
		ShowDataplaneGraph: true,
		ShowFooter:         true,
	}
}

func sanitizeUIPreferences(p UIPreferences) UIPreferences {
	if p.Theme == ThemeOption("auto") {
		p.Theme = ThemeLight
	}
	if migrated, ok := legacyThemeMigration[p.Theme]; ok {
		p.Theme = migrated
	}
	if !isKnownTheme(p.Theme) {
		p.Theme = ThemeLight
	}
	if strings.TrimSpace(p.Language) == "" {
		p.Language = "en"
	}
	if p.StatsUnits != StatsUnitsBytes && p.StatsUnits != StatsUnitsBiBytes {
		p.StatsUnits = StatsUnitsBiBytes
	}
	return p
}

func isKnownTheme(theme ThemeOption) bool {
	for _, option := range orderedThemeOptions {
		if option == theme {
			return true
		}
	}
	return false
}

func uiPreferencesPath() string {
	custom := strings.TrimSpace(os.Getenv(uiSettingsPathEnv))
	if custom != "" {
		return custom
	}
	return defaultUIPreferencesPath()
}

func defaultUIPreferencesPath() string {
	if runtimeGOOS == "windows" {
		programData := strings.TrimSpace(os.Getenv("ProgramData"))
		if programData == "" {
			programData = `C:\ProgramData`
		}
		return filepath.Join(programData, "TunGo", "tui.json")
	}
	return filepath.Join(string(os.PathSeparator), "etc", "tungo", "tui.json")
}

func loadUIPreferencesFromDisk() (UIPreferences, error) {
	path := uiPreferencesPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return defaultUIPreferences(), nil
		}
		return defaultUIPreferences(), err
	}

	var p UIPreferences
	if err := json.Unmarshal(data, &p); err != nil {
		return defaultUIPreferences(), err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return defaultUIPreferences(), err
	}
	if _, ok := raw["show_dataplane_stats"]; !ok {
		p.ShowDataplaneStats = true
	}
	if _, ok := raw["show_dataplane_graph"]; !ok {
		p.ShowDataplaneGraph = true
	}
	return sanitizeUIPreferences(p), nil
}

func persistUIPreferencesToDisk(p UIPreferences) error {
	path := uiPreferencesPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	payload, err := marshalUIPreferences(sanitizeUIPreferences(p))
	if err != nil {
		return err
	}
	payload = append(payload, '\n')

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, payload, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}
