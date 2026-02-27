package bubble_tea

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"tungo/infrastructure/PAL/configuration/client"
)

type prefsStorage interface {
	Read() ([]byte, error)
	Write(data []byte) error
}

type defaultPrefsStorage struct {
	filePath string
}

func newDefaultPrefsStorage() defaultPrefsStorage {
	base, _ := client.DefaultResolver{}.Resolve()
	return defaultPrefsStorage{
		filePath: filepath.Join(filepath.Dir(base), "tui.json"),
	}
}

func (s defaultPrefsStorage) Read() ([]byte, error) {
	data, err := os.ReadFile(s.filePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

func (s defaultPrefsStorage) Write(data []byte) error {
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0o711); err != nil {
		return err
	}

	tmpPath := s.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, s.filePath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func loadPreferences(s prefsStorage) (UIPreferences, error) {
	defaults := newUIPreferences(ThemeLight, "en", StatsUnitsBiBytes)

	data, err := s.Read()
	if err != nil {
		return defaults, err
	}
	if data == nil {
		return defaults, nil
	}

	var p UIPreferences
	if err := json.Unmarshal(data, &p); err != nil {
		return defaults, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return defaults, err
	}

	result := newUIPreferences(p.Theme, p.Language, p.StatsUnits)
	if _, ok := raw["show_dataplane_stats"]; ok {
		result.ShowDataplaneStats = p.ShowDataplaneStats
	}
	if _, ok := raw["show_dataplane_graph"]; ok {
		result.ShowDataplaneGraph = p.ShowDataplaneGraph
	}
	if _, ok := raw["show_footer"]; ok {
		result.ShowFooter = p.ShowFooter
	}
	if _, ok := raw["preferred_mode"]; ok && isValidModePreference(p.PreferredMode) {
		result.PreferredMode = p.PreferredMode
	}
	if _, ok := raw["auto_connect"]; ok {
		result.AutoConnect = p.AutoConnect
	}
	if _, ok := raw["last_client_config"]; ok {
		result.LastClientConfig = p.LastClientConfig
	}
	return result, nil
}

func savePreferencesTo(s prefsStorage, p UIPreferences) error {
	payload, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return s.Write(append(payload, '\n'))
}

func savePreferencesToDisk(p UIPreferences) error {
	return savePreferencesTo(newDefaultPrefsStorage(), p)
}

func loadUISettingsFromDisk() *uiPreferencesProvider {
	loaded, err := loadPreferences(newDefaultPrefsStorage())
	if err != nil {
		return newDefaultUIPreferencesProvider()
	}
	return newUIPreferencesProvider(loaded)
}
