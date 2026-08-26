package tui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"tungo/internal/config/internal/configpath"
)

func Load() (Configuration, error) {
	return load(filepath.Join(configpath.Directory(), "tui.json"))
}

func load(path string) (Configuration, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Default(), err
	}

	var persisted Configuration
	if err := json.Unmarshal(data, &persisted); err != nil {
		return Default(), err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return Default(), err
	}

	configuration := normalize(persisted)
	defaults := Default()
	if _, ok := fields["show_dataplane_stats"]; !ok {
		configuration.ShowDataplaneStats = defaults.ShowDataplaneStats
	}
	if _, ok := fields["show_dataplane_graph"]; !ok {
		configuration.ShowDataplaneGraph = defaults.ShowDataplaneGraph
	}
	if _, ok := fields["show_footer"]; !ok {
		configuration.ShowFooter = defaults.ShowFooter
	}
	if _, ok := fields["auto_select_mode"]; !ok || !validModePreference(persisted.AutoSelectMode) {
		configuration.AutoSelectMode = legacyModePreference(fields, defaults.AutoSelectMode)
	}
	if _, ok := fields["auto_select_client_config"]; !ok {
		configuration.AutoSelectClientConfig = legacyClientConfiguration(fields)
	}
	return configuration, nil
}

func legacyModePreference(fields map[string]json.RawMessage, fallback ModePreference) ModePreference {
	var value ModePreference
	if raw, ok := fields["preferred_mode"]; ok && json.Unmarshal(raw, &value) == nil && validModePreference(value) {
		return value
	}
	return fallback
}

func legacyClientConfiguration(fields map[string]json.RawMessage) string {
	var value string
	if raw, ok := fields["last_client_config"]; ok && json.Unmarshal(raw, &value) == nil {
		return value
	}
	return ""
}

func Save(configuration Configuration) error {
	return save(filepath.Join(configpath.Directory(), "tui.json"), configuration)
}

func save(path string, configuration Configuration) error {
	data, err := json.MarshalIndent(configuration, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0o711); err != nil {
		return err
	}
	temporaryPath := path + ".tmp"
	if err := os.WriteFile(temporaryPath, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}
	return nil
}
