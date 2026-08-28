package tui

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.json")
	want := Configuration{
		Theme:                  ThemeDark,
		Language:               "en",
		StatsUnits:             StatsUnitsBytes,
		ShowDataplaneStats:     true,
		ShowDataplaneGraph:     true,
		ShowFooter:             true,
		AutoSelectMode:         ModePreferenceClient,
		AutoConnect:            true,
		AutoSelectClientConfig: "office",
	}
	if err := save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("load() = %+v, want %+v", got, want)
	}
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	got, err := load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got != Default() {
		t.Fatalf("load() = %+v, want defaults", got)
	}
}

func TestLoadInvalidJSONReturnsDefaultsAndError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.json")
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := load(path)
	if err == nil {
		t.Fatal("expected JSON error")
	}
	if got != Default() {
		t.Fatalf("load() = %+v, want defaults", got)
	}
}

func TestLoadNormalizesAndAppliesMissingDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.json")
	writeTestConfiguration(t, path, `{
		"theme":"unsupported",
		"language":"",
		"stats_units":"unsupported",
		"auto_select_mode":"unsupported"
	}`)
	got, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != Default() {
		t.Fatalf("load() = %+v, want defaults", got)
	}
}

func TestLoadPreservesExplicitFalseValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.json")
	writeTestConfiguration(t, path, `{
		"show_dataplane_stats":false,
		"show_dataplane_graph":false,
		"show_footer":false
	}`)
	got, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.ShowDataplaneStats || got.ShowDataplaneGraph || got.ShowFooter {
		t.Fatalf("explicit false values were replaced: %+v", got)
	}
}

func TestLoadMigratesLegacyFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.json")
	writeTestConfiguration(t, path, `{
		"preferred_mode":"client",
		"last_client_config":"office"
	}`)
	got, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.AutoSelectMode != ModePreferenceClient || got.AutoSelectClientConfig != "office" {
		t.Fatalf("legacy fields were not migrated: %+v", got)
	}
}

func TestLoadUsesLegacyModeWhenNewValueIsInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.json")
	writeTestConfiguration(t, path, `{
		"auto_select_mode":"unsupported",
		"preferred_mode":"server"
	}`)
	got, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.AutoSelectMode != ModePreferenceServer {
		t.Fatalf("AutoSelectMode = %q, want server", got.AutoSelectMode)
	}
}

func TestLoadReadError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.json")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := load(path); err == nil {
		t.Fatal("expected read error")
	}
}

func TestSaveErrors(t *testing.T) {
	t.Run("create directory", func(t *testing.T) {
		parent := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(parent, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := save(filepath.Join(parent, "tui.json"), Default()); err == nil {
			t.Fatal("expected directory creation error")
		}
	})

	t.Run("write temporary file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "tui.json")
		if err := os.Mkdir(path+".tmp", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := save(path, Default()); err == nil {
			t.Fatal("expected temporary file error")
		}
	})

	t.Run("rename", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "tui.json")
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := save(path, Default()); err == nil {
			t.Fatal("expected rename error")
		}
		if _, err := os.Stat(path + ".tmp"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("temporary file remains after rename error: %v", err)
		}
	})
}

func writeTestConfiguration(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
