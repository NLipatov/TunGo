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
	ThemeLight ThemeOption = "light"
	ThemeDark  ThemeOption = "dark"
)

type StatsUnitsOption string

const (
	StatsUnitsBytes   StatsUnitsOption = "bytes"
	StatsUnitsBiBytes StatsUnitsOption = "bibytes"
)

type UIPreferences struct {
	Theme      ThemeOption      `json:"theme"`
	Language   string           `json:"language"`
	StatsUnits StatsUnitsOption `json:"stats_units"`
	ShowFooter bool             `json:"show_footer"`
}

var (
	preferences atomic.Value
	prefsMu     sync.Mutex
)

const uiSettingsPathEnv = "TUNGO_UI_SETTINGS_PATH"

func init() {
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
		Theme:      ThemeLight,
		Language:   "en",
		StatsUnits: StatsUnitsBiBytes,
		ShowFooter: true,
	}
}

func sanitizeUIPreferences(p UIPreferences) UIPreferences {
	if p.Theme == ThemeOption("auto") {
		p.Theme = ThemeLight
	}
	if p.Theme != ThemeLight && p.Theme != ThemeDark {
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

func uiPreferencesPath() string {
	custom := strings.TrimSpace(os.Getenv(uiSettingsPathEnv))
	if custom != "" {
		return custom
	}
	return defaultUIPreferencesPath()
}

func defaultUIPreferencesPath() string {
	if runtime.GOOS == "windows" {
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
	return sanitizeUIPreferences(p), nil
}

func persistUIPreferencesToDisk(p UIPreferences) error {
	path := uiPreferencesPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	payload, err := json.MarshalIndent(sanitizeUIPreferences(p), "", "  ")
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
