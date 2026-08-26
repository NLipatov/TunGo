package client

import (
	"encoding/json"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"tungo/internal/config/settings"
)

func validTestConfiguration() Configuration {
	return Configuration{
		ClientID: 1,
		UDPSettings: settings.Settings{
			Network: settings.Network{
				TunName:    "tun0",
				Server:     settings.Host{IPv4: "127.0.0.1"},
				Port:       9090,
				IPv4Subnet: netip.MustParsePrefix("10.0.1.0/24"),
			},
			Protocol: settings.UDP,
		},
		X25519PublicKey:  make([]byte, 32),
		ClientPublicKey:  make([]byte, 32),
		ClientPrivateKey: make([]byte, 32),
		Protocol:         settings.UDP,
	}
}

func writeConfiguration(t *testing.T, path string, configuration Configuration) {
	t.Helper()
	data, err := json.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestConfigurationsActiveAppliesDefaultsAndValidates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client_configuration.json")
	writeConfiguration(t, path, validTestConfiguration())

	configuration, err := (&Configurations{activePath: path}).Active()
	if err != nil {
		t.Fatal(err)
	}
	if configuration.UDPSettings.MTU != settings.DefaultMTU {
		t.Fatalf("MTU = %d, want %d", configuration.UDPSettings.MTU, settings.DefaultMTU)
	}
	active, err := configuration.ActiveSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !active.IPv4.IsValid() {
		t.Fatal("active client address was not derived")
	}
}

func TestConfigurationsActiveErrors(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		_, err := (&Configurations{activePath: filepath.Join(t.TempDir(), "missing.json")}).Active()
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("error = %v, want os.ErrNotExist", err)
		}
	})
	t.Run("invalid JSON", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "client_configuration.json")
		if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := (&Configurations{activePath: path}).Active(); err == nil {
			t.Fatal("expected decode error")
		}
	})
	t.Run("invalid configuration", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "client_configuration.json")
		writeConfiguration(t, path, Configuration{})
		if _, err := (&Configurations{activePath: path}).Active(); err == nil || !strings.Contains(err.Error(), "invalid client configuration") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestConfigurationsListActivateAndDelete(t *testing.T) {
	directory := t.TempDir()
	activePath := filepath.Join(directory, "client_configuration.json")
	configurations := &Configurations{activePath: activePath}
	writeConfiguration(t, activePath, validTestConfiguration())
	writeConfiguration(t, activePath+".first", validTestConfiguration())

	second := validTestConfiguration()
	second.ClientID = 7
	writeConfiguration(t, activePath+".second", second)
	if err := os.WriteFile(filepath.Join(directory, "unrelated.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(activePath+".directory", 0700); err != nil {
		t.Fatal(err)
	}

	listed, err := configurations.List()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(listed, []string{"first", "second"}) {
		t.Fatalf("List() = %v", listed)
	}

	if err := configurations.Activate("second"); err != nil {
		t.Fatal(err)
	}
	var persisted Configuration
	data, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.UDPSettings.MTU != settings.DefaultMTU {
		t.Fatalf("persisted MTU = %d, want %d", persisted.UDPSettings.MTU, settings.DefaultMTU)
	}
	active, err := configurations.Active()
	if err != nil {
		t.Fatal(err)
	}
	if active.ClientID != 7 {
		t.Fatalf("ClientID = %d, want 7", active.ClientID)
	}

	if err := configurations.Delete("first"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(activePath + ".first"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted file stat error = %v", err)
	}
}

func TestConfigurationsListMissingDirectory(t *testing.T) {
	configurations := &Configurations{activePath: filepath.Join(t.TempDir(), "missing", "client_configuration.json")}
	listed, err := configurations.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("List() = %v, want empty", listed)
	}
}

func TestConfigurationsImportNormalizesAndRejectsInvalidInput(t *testing.T) {
	activePath := filepath.Join(t.TempDir(), "nested", "client_configuration.json")
	configurations := &Configurations{activePath: activePath}
	data, err := json.Marshal(validTestConfiguration())
	if err != nil {
		t.Fatal(err)
	}

	if err := configurations.Import("office", "\ufeff\u200b"+string(data)+"\r\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeFile(activePath + ".office"); err != nil {
		t.Fatalf("decode imported configuration: %v", err)
	}

	for _, name := range []string{"", "../escape", `..\escape`, ".", "..", "bad\x00name", "office:backup"} {
		if err := configurations.Import(name, string(data)); err == nil {
			t.Fatalf("Import(%q) succeeded", name)
		}
	}
	if err := configurations.Import("broken", "{"); err == nil {
		t.Fatal("invalid JSON was accepted")
	}
}

func TestConfigurationsActivateInvalidAlternativePreservesActive(t *testing.T) {
	activePath := filepath.Join(t.TempDir(), "client_configuration.json")
	configurations := &Configurations{activePath: activePath}
	writeConfiguration(t, activePath, validTestConfiguration())
	before, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(activePath+".broken", []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}

	err = configurations.Activate("broken")
	if err == nil || !strings.Contains(err.Error(), "invalid client configuration") {
		t.Fatalf("Activate() error = %v", err)
	}
	after, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(before, after) {
		t.Fatal("invalid alternative replaced the active configuration")
	}
}

func decodeFile(path string) (Configuration, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Configuration{}, err
	}
	return decode(data)
}
