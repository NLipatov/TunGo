package configuration

import (
	"encoding/json"
	"errors"
	"net/netip"
	"strings"
	"testing"

	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/settings"
)

type clientObserverFunc func() ([]string, error)

func (f clientObserverFunc) Observe() ([]string, error) { return f() }

type clientSelectorFunc func(string) error

func (f clientSelectorFunc) Select(path string) error { return f(path) }

type clientCreatorFunc func(clientConfiguration.Configuration, string) error

func (f clientCreatorFunc) Create(cfg clientConfiguration.Configuration, name string) error {
	return f(cfg, name)
}

type clientDeleterFunc func(string) error

func (f clientDeleterFunc) Delete(path string) error { return f(path) }

func mustHostParser(raw string) settings.Host {
	h, err := settings.NewHost(raw)
	if err != nil {
		panic(err)
	}
	return h
}

func makeTestConfig() clientConfiguration.Configuration {
	return clientConfiguration.Configuration{
		ClientID: 1,
		TCPSettings: settings.Settings{
			Addressing: settings.Addressing{
				TunName:    "tun0",
				Server:     mustHostParser("127.0.0.1"),
				Port:       8080,
				IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24"),
			},
			Protocol: settings.TCP,
		},
		X25519PublicKey:  make([]byte, 32),
		ClientPublicKey:  make([]byte, 32),
		ClientPrivateKey: make([]byte, 32),
		Protocol:         settings.TCP,
	}
}

func TestParseClientConfigurationJSON_Simple(t *testing.T) {
	want := makeTestConfig()
	raw, _ := json.Marshal(want)

	cfg, err := parseClientConfigurationJSON(string(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TCPSettings.Server != want.TCPSettings.Server || cfg.TCPSettings.Port != want.TCPSettings.Port {
		t.Errorf("got %+v, want %+v", cfg, want)
	}
	if cfg.Protocol != want.Protocol {
		t.Errorf("got Protocol=%v, want %v", cfg.Protocol, want.Protocol)
	}
}

func TestParseClientConfigurationJSON_WithBOMAndZeroWidthAndControl(t *testing.T) {
	want := makeTestConfig()
	raw, _ := json.Marshal(want)
	dirty := "\uFEFF\u200B\x00\x07  " + string(raw) + "  \x0B\u200B\uFEFF"

	cfg, err := parseClientConfigurationJSON(dirty)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TCPSettings.Port != want.TCPSettings.Port {
		t.Errorf("got Port=%d, want %d", cfg.TCPSettings.Port, want.TCPSettings.Port)
	}
}

func TestParseClientConfigurationJSON_PrettyPrintCRLF(t *testing.T) {
	want := makeTestConfig()
	raw, _ := json.MarshalIndent(want, "", "  ")
	pretty := strings.ReplaceAll(string(raw), "\n", "\r\n")

	cfg, err := parseClientConfigurationJSON(pretty)
	if err != nil {
		t.Fatalf("failed to parse CRLF JSON: %v", err)
	}
	if cfg.Protocol != settings.TCP {
		t.Errorf("got Protocol=%v, want %v", cfg.Protocol, settings.TCP)
	}
}

func TestParseClientConfigurationJSON_NonBreakingSpaceTrim(t *testing.T) {
	want := makeTestConfig()
	raw, _ := json.Marshal(want)
	dirty := "\u00A0\u00A0" + string(raw) + "\u00A0"

	cfg, err := parseClientConfigurationJSON(dirty)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TCPSettings.Server != want.TCPSettings.Server {
		t.Errorf("got Server=%q, want %q", cfg.TCPSettings.Server, want.TCPSettings.Server)
	}
}

func TestParseClientConfigurationJSON_Invalid(t *testing.T) {
	_, err := parseClientConfigurationJSON("not a valid { json")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestParseClientConfigurationJSON_InvalidByValidation(t *testing.T) {
	cfg := makeTestConfig()
	cfg.ClientID = 0
	raw, _ := json.Marshal(cfg)

	_, err := parseClientConfigurationJSON(string(raw))
	if err == nil {
		t.Fatal("expected validation error for invalid client configuration")
	}
}

func TestClientControlDelegates(t *testing.T) {
	wantErr := errors.New("delegate failed")
	selectedPath := ""
	createdName := ""
	deletedPath := ""
	control := clientControl{
		observer: clientObserverFunc(func() ([]string, error) {
			return []string{"a", "b"}, nil
		}),
		selector: clientSelectorFunc(func(path string) error {
			selectedPath = path
			return wantErr
		}),
		creator: clientCreatorFunc(func(_ clientConfiguration.Configuration, name string) error {
			createdName = name
			return nil
		}),
		deleter: clientDeleterFunc(func(path string) error {
			deletedPath = path
			return nil
		}),
		manager: runtimeInfoClientManager{cfg: &clientConfiguration.Configuration{}},
	}

	list, err := control.List()
	if err != nil || len(list) != 2 {
		t.Fatalf("List() = %v, %v", list, err)
	}
	if err := control.Select("selected.json"); !errors.Is(err, wantErr) {
		t.Fatalf("Select() error = %v, want %v", err, wantErr)
	}
	if selectedPath != "selected.json" {
		t.Fatalf("selected path = %q", selectedPath)
	}

	raw, _ := json.Marshal(makeTestConfig())
	if err := control.CreateFromJSON("new-client", string(raw)); err != nil {
		t.Fatalf("CreateFromJSON() error = %v", err)
	}
	if createdName != "new-client" {
		t.Fatalf("created name = %q", createdName)
	}
	if err := control.Delete("old.json"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if deletedPath != "old.json" {
		t.Fatalf("deleted path = %q", deletedPath)
	}
}

func TestClientControlCreateFromJSON_InvalidInput(t *testing.T) {
	creatorCalled := false
	control := clientControl{
		creator: clientCreatorFunc(func(clientConfiguration.Configuration, string) error {
			creatorCalled = true
			return nil
		}),
	}

	err := control.CreateFromJSON("bad-client", "not json")
	if err == nil || !strings.Contains(err.Error(), "invalid client configuration") {
		t.Fatalf("expected invalid configuration error, got %v", err)
	}
	if creatorCalled {
		t.Fatal("expected creator not to be called for invalid JSON")
	}
}

func TestClientControlValidateActive(t *testing.T) {
	wantErr := errors.New("read failed")
	control := clientControl{manager: runtimeInfoClientManager{err: wantErr}}

	if err := control.ValidateActive(); !errors.Is(err, wantErr) {
		t.Fatalf("ValidateActive() error = %v, want %v", err, wantErr)
	}
}
