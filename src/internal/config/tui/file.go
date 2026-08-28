package tui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"tungo/internal/config/internal/configpath"
)

// Load reads the TUI configuration from the configured directory, returning default settings when the configuration file is unavailable.
func Load() (Configuration, error) {
	return load(filepath.Join(configpath.Directory(), "tui.json"))
}

// load reads and migrates a persisted configuration from path.
// It returns the default configuration when the file is absent and returns
// read or malformed-data errors with the default configuration. Missing fields
// are restored from defaults or migrated from legacy configuration fields.
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

// legacyModePreference returns the valid legacy mode preference from fields, or fallback when the preference is absent or invalid.
func legacyModePreference(fields map[string]json.RawMessage, fallback ModePreference) ModePreference {
	var value ModePreference
	if raw, ok := fields["preferred_mode"]; ok && json.Unmarshal(raw, &value) == nil && validModePreference(value) {
		return value
	}
	return fallback
}

// legacyClientConfiguration retrieves the legacy client configuration value.
// It returns an empty string when the field is absent or cannot be unmarshaled.
func legacyClientConfiguration(fields map[string]json.RawMessage) string {
	var value string
	if raw, ok := fields["last_client_config"]; ok && json.Unmarshal(raw, &value) == nil {
		return value
	}
	return ""
}

// Save persists the TUI configuration to the configured directory.
func Save(configuration Configuration) error {
	return save(filepath.Join(configpath.Directory(), "tui.json"), configuration)
}

// save writes the configuration as indented JSON to path, creating its parent directory and atomically replacing any existing file.
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
