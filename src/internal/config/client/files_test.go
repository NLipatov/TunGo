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
	if want := []string{"1.1.1.1", "8.8.8.8"}; !slices.Equal(configuration.UDPSettings.DNSv4, want) {
		t.Fatalf("DNSv4 = %v, want %v", configuration.UDPSettings.DNSv4, want)
	}
	if len(configuration.UDPSettings.DNSv6) != 0 {
		t.Fatalf("DNSv6 = %v, want no IPv6 resolvers", configuration.UDPSettings.DNSv6)
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
	t.Run("unreadable", func(t *testing.T) {
		_, err := (&Configurations{activePath: t.TempDir()}).Active()
		if err == nil || !strings.Contains(err.Error(), "failed to read client configuration") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestConfigurationsActiveNormalizesAllowedIPs(t *testing.T) {
	configuration := validTestConfiguration()
	configuration.AllowedIPsv4 = []string{
		"10.20.1.99/24", "192.0.2.42/24", "10.20.1.0/24", "192.0.2.42/24",
	}
	configuration.AllowedIPsv6 = []string{
		"2001:db8:2::99/64", "2001:db8:1::42/64", "2001:0db8:0002::/64", "2001:db8:1::42/64",
	}
	path := filepath.Join(t.TempDir(), "client_configuration.json")
	writeConfiguration(t, path, configuration)

	loaded, err := (&Configurations{activePath: path}).Active()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"10.20.1.0/24", "192.0.2.0/24"}; !slices.Equal(loaded.AllowedIPsv4, want) {
		t.Errorf("AllowedIPsv4 = %v, want %v", loaded.AllowedIPsv4, want)
	}
	if want := []string{"2001:db8:2::/64", "2001:db8:1::/64"}; !slices.Equal(loaded.AllowedIPsv6, want) {
		t.Errorf("AllowedIPsv6 = %v, want %v", loaded.AllowedIPsv6, want)
	}
}

func TestConfigurationsAllowedIPsPreserveEmptyLists(t *testing.T) {
	configuration := validTestConfiguration()
	configuration.AllowedIPsv4 = []string{}
	configuration.AllowedIPsv6 = []string{}
	data, err := json.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	configurations := &Configurations{activePath: filepath.Join(t.TempDir(), "client_configuration.json")}
	if err := configurations.Import("empty", string(data)); err != nil {
		t.Fatal(err)
	}
	if err := configurations.Activate("empty"); err != nil {
		t.Fatal(err)
	}
	loaded, err := configurations.Active()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AllowedIPsv4 == nil || len(loaded.AllowedIPsv4) != 0 {
		t.Errorf("AllowedIPsv4 = %#v, want a non-nil empty list", loaded.AllowedIPsv4)
	}
	if loaded.AllowedIPsv6 == nil || len(loaded.AllowedIPsv6) != 0 {
		t.Errorf("AllowedIPsv6 = %#v, want a non-nil empty list", loaded.AllowedIPsv6)
	}
}

func TestConfigurationsActiveDefaultsInvalidAllowedIPs(t *testing.T) {
	defaultV4 := []string{"0.0.0.0/1", "128.0.0.0/1"}
	defaultV6 := []string{"::/1", "8000::/1"}
	customV4 := []string{"192.0.2.0/24"}
	customV6 := []string{"2001:db8::/64"}
	for _, tt := range []struct {
		name   string
		v4     []string
		v6     []string
		wantV4 []string
		wantV6 []string
	}{
		{name: "missing lists", wantV4: defaultV4, wantV6: defaultV6},
		{name: "invalid IPv4 CIDR", v4: []string{"10.0.0.0/33"}, v6: customV6, wantV4: defaultV4, wantV6: customV6},
		{name: "invalid IPv6 CIDR", v4: customV4, v6: []string{"2001:db8::/129"}, wantV4: customV4, wantV6: defaultV6},
		{name: "IPv6 in IPv4 list", v4: customV6, v6: customV6, wantV4: defaultV4, wantV6: customV6},
		{name: "IPv4 in IPv6 list", v4: customV4, v6: customV4, wantV4: customV4, wantV6: defaultV6},
		{name: "mapped IPv4 in IPv4 list", v4: []string{"::ffff:192.0.2.0/120"}, v6: customV6, wantV4: defaultV4, wantV6: customV6},
		{name: "mapped IPv4 in IPv6 list", v4: customV4, v6: []string{"::ffff:192.0.2.0/120"}, wantV4: customV4, wantV6: defaultV6},
		{name: "partially invalid IPv4 list", v4: []string{"192.0.2.1/24", "bad CIDR"}, v6: customV6, wantV4: defaultV4, wantV6: customV6},
		{name: "partially invalid IPv6 list", v4: customV4, v6: []string{"2001:db8::1/64", "bad CIDR"}, wantV4: customV4, wantV6: defaultV6},
		{name: "invalid IPv4 preserves empty IPv6", v4: []string{"bad CIDR"}, v6: []string{}, wantV4: defaultV4, wantV6: []string{}},
		{name: "invalid IPv6 preserves empty IPv4", v4: []string{}, v6: []string{"bad CIDR"}, wantV4: []string{}, wantV6: defaultV6},
		{name: "both lists invalid", v4: []string{"bad CIDR"}, v6: []string{"bad CIDR"}, wantV4: defaultV4, wantV6: defaultV6},
	} {
		t.Run(tt.name, func(t *testing.T) {
			configuration := validTestConfiguration()
			configuration.AllowedIPsv4 = tt.v4
			configuration.AllowedIPsv6 = tt.v6
			path := filepath.Join(t.TempDir(), "client_configuration.json")
			writeConfiguration(t, path, configuration)
			loaded, err := (&Configurations{activePath: path}).Active()
			if err != nil {
				t.Fatal(err)
			}
			if loaded.AllowedIPsv4 == nil || !slices.Equal(loaded.AllowedIPsv4, tt.wantV4) {
				t.Errorf("AllowedIPsv4 = %#v, want %#v", loaded.AllowedIPsv4, tt.wantV4)
			}
			if loaded.AllowedIPsv6 == nil || !slices.Equal(loaded.AllowedIPsv6, tt.wantV6) {
				t.Errorf("AllowedIPsv6 = %#v, want %#v", loaded.AllowedIPsv6, tt.wantV6)
			}
		})
	}
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
	if err := os.WriteFile(activePath+"malformed", []byte("{}"), 0600); err != nil {
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

func TestConfigurationsListReturnsReadError(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(parent, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	configurations := &Configurations{activePath: filepath.Join(parent, "client_configuration.json")}
	if _, err := configurations.List(); err == nil {
		t.Fatal("List() succeeded when the configuration directory was unreadable")
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

func TestConfigurationsImportReturnsStorageErrors(t *testing.T) {
	data, err := json.Marshal(validTestConfiguration())
	if err != nil {
		t.Fatal(err)
	}

	t.Run("create directory", func(t *testing.T) {
		parent := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(parent, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
		configurations := &Configurations{activePath: filepath.Join(parent, "client_configuration.json")}
		if err := configurations.Import("office", string(data)); err == nil {
			t.Fatal("Import() succeeded when its directory could not be created")
		}
	})

	t.Run("write alternative", func(t *testing.T) {
		activePath := filepath.Join(t.TempDir(), "client_configuration.json")
		if err := os.Mkdir(activePath+".office", 0700); err != nil {
			t.Fatal(err)
		}
		if err := (&Configurations{activePath: activePath}).Import("office", string(data)); err == nil {
			t.Fatal("Import() succeeded when the alternative path was a directory")
		}
	})
}

func TestConfigurationsImportNameRules(t *testing.T) {
	data, err := json.Marshal(validTestConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "Abc_0123456789-Z"} {
		t.Run("accept_"+name, func(t *testing.T) {
			activePath := filepath.Join(t.TempDir(), "client_configuration.json")
			configurations := &Configurations{activePath: activePath}
			if err := configurations.Import(name, string(data)); err != nil {
				t.Fatal(err)
			}
			if _, err := decodeFile(activePath + "." + name); err != nil {
				t.Fatalf("read imported configuration: %v", err)
			}
		})
	}
	for _, name := range []string{
		strings.Repeat("a", 17), "office vpn", " office", "office ",
		"office.backup", "сервер", "office\tbackup", "office\nbackup",
		"office\x00backup", `{"ClientID":1}`, "{", "", ".", "..", "../office",
		`office\backup`, "office:backup", "office*", "office?", "office|", "<office>",
	} {
		t.Run("reject_"+name, func(t *testing.T) {
			directory := t.TempDir()
			configurations := &Configurations{activePath: filepath.Join(directory, "client_configuration.json")}
			if err := configurations.Import(name, string(data)); err == nil {
				t.Fatalf("Import(%q) accepted an invalid name", name)
			}
			entries, err := os.ReadDir(directory)
			if err != nil || len(entries) != 0 {
				t.Fatalf("invalid import changed storage: entries=%v, err=%v", entries, err)
			}
		})
	}
}

func TestConfigurationsLegacyNamesRemainAccessible(t *testing.T) {
	activePath := filepath.Join(t.TempDir(), "client_configuration.json")
	configurations := &Configurations{activePath: activePath}
	name := "Мой office.backup configuration"
	configuration := validTestConfiguration()
	writeConfiguration(t, activePath+"."+name, configuration)

	names, err := configurations.List()
	if err != nil || !slices.Equal(names, []string{name}) {
		t.Fatalf("List() = %v, %v", names, err)
	}
	if err := configurations.Activate(name); err != nil {
		t.Fatalf("activate legacy configuration: %v", err)
	}
	active, err := configurations.Active()
	if err != nil || active.ClientID != configuration.ClientID {
		t.Fatalf("Active() = %v, %v", active, err)
	}
	if err := configurations.Delete(name); err != nil {
		t.Fatalf("delete legacy configuration: %v", err)
	}
	if _, err := os.Stat(activePath + "." + name); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy configuration still exists: %v", err)
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

func TestConfigurationsActivateReturnsStorageErrors(t *testing.T) {
	t.Run("invalid name", func(t *testing.T) {
		configurations := &Configurations{activePath: filepath.Join(t.TempDir(), "client_configuration.json")}
		if err := configurations.Activate("../office"); err == nil {
			t.Fatal("Activate() accepted an invalid name")
		}
	})

	t.Run("missing alternative", func(t *testing.T) {
		configurations := &Configurations{activePath: filepath.Join(t.TempDir(), "client_configuration.json")}
		if err := configurations.Activate("missing"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("Activate() error = %v, want os.ErrNotExist", err)
		}
	})

	t.Run("write active", func(t *testing.T) {
		activePath := filepath.Join(t.TempDir(), "client_configuration.json")
		if err := os.Mkdir(activePath, 0700); err != nil {
			t.Fatal(err)
		}
		writeConfiguration(t, activePath+".office", validTestConfiguration())
		if err := (&Configurations{activePath: activePath}).Activate("office"); err == nil {
			t.Fatal("Activate() succeeded when the active path was a directory")
		}
	})
}

func TestConfigurationsDeleteRejectsInvalidName(t *testing.T) {
	configurations := &Configurations{activePath: filepath.Join(t.TempDir(), "client_configuration.json")}
	if err := configurations.Delete("../office"); err == nil {
		t.Fatal("Delete() accepted an invalid name")
	}
}

func decodeFile(path string) (Configuration, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Configuration{}, err
	}
	return decode(data)
}
