package tui

import (
	"context"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"

	clientconfig "tungo/internal/config/client"
	serverconfig "tungo/internal/config/server"
	"tungo/internal/config/settings"
	"tungo/internal/mode"
	bubbleTea "tungo/internal/ui/tui/internal/bubble_tea"

	tea "charm.land/bubbletea/v2"
)

func TestNewTUI(t *testing.T) {
	directory := t.TempDir()
	ui, err := New(
		clientconfig.Files(),
		serverconfig.NewFile(filepath.Join(directory, "server_configuration.json")),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if ui.clientConfigurations == nil || ui.serverFile == nil {
		t.Fatal("configuration files were not registered")
	}
}

func TestNewTUIRejectsMissingClientConfigurations(t *testing.T) {
	ui, err := New(nil, nil, nil)
	if err == nil || ui != nil {
		t.Fatalf("New() = %v, %v; want nil and error", ui, err)
	}
}

func TestTUIRunCanceledContextReturnsNil(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := newTestTUI(t).Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestTUIRunWrapsConfigurationErrorWithoutTTY(t *testing.T) {
	requireNoTTY(t)
	err := newTestTUI(t).Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "configuration error") {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestTUIConfigurePropagatesProgramRunErrorWithoutTTY(t *testing.T) {
	requireNoTTY(t)
	_, err := newTestTUI(t).configure(context.Background(), bubbleTea.NewRuntimeLogBuffer(8))
	if err == nil || !strings.Contains(err.Error(), "opening TTY") {
		t.Fatalf("configure() error = %v", err)
	}
}

func TestTUIRunRuntimeRejectsInvalidMode(t *testing.T) {
	err := newTestTUI(t).runRuntime(context.Background(), 0, bubbleTea.NewRuntimeLogBuffer(8))
	if err == nil || err.Error() != "invalid runtime mode: 0" {
		t.Fatalf("runRuntime() error = %v", err)
	}
}

func TestTUIRunRuntimeReportsServerConfigurationErrors(t *testing.T) {
	logBuffer := bubbleTea.NewRuntimeLogBuffer(8)

	t.Run("missing file", func(t *testing.T) {
		ui := newTestTUI(t)
		ui.serverFile = nil
		err := ui.runRuntime(context.Background(), mode.Server, logBuffer)
		if err == nil || !strings.Contains(err.Error(), "server configuration file is nil") {
			t.Fatalf("runRuntime() error = %v", err)
		}
	})

	t.Run("unreadable file", func(t *testing.T) {
		ui := newTestTUI(t)
		ui.serverFile = serverconfig.NewFile(t.TempDir())
		err := ui.runRuntime(context.Background(), mode.Server, logBuffer)
		if err == nil || !strings.Contains(err.Error(), "load server configuration") {
			t.Fatalf("runRuntime() error = %v", err)
		}
	})
}

func TestTUIRunRuntimePhasePropagatesProgramRunErrorWithoutTTY(t *testing.T) {
	requireNoTTY(t)
	reconfigure, err := newTestTUI(t).runRuntimePhase(context.Background(), bubbleTea.RuntimeDashboardOptions{Mode: mode.Client})
	if err == nil || !strings.Contains(err.Error(), "opening TTY") || reconfigure {
		t.Fatalf("runRuntimePhase() = %v, %v", reconfigure, err)
	}
}

func TestShowFatalErrorReturnsWhenProgramCannotOpenTTY(t *testing.T) {
	requireNoTTY(t)
	ShowFatalError("fatal")
}

func TestClientRuntimeInfo(t *testing.T) {
	configuration := validClientConfiguration()
	info, err := clientRuntimeInfo(configuration)
	if err != nil {
		t.Fatal(err)
	}
	if info.Protocol != settings.TCP || len(info.Endpoints) != 1 || info.Endpoints[0].Port != 8080 {
		t.Fatalf("runtime info = %+v", info)
	}
}

func TestClientRuntimeInfoRejectsInvalidConfiguration(t *testing.T) {
	configuration := validClientConfiguration()
	configuration.Protocol = settings.UNKNOWN
	if _, err := clientRuntimeInfo(configuration); err == nil {
		t.Fatal("clientRuntimeInfo() accepted an invalid protocol")
	}
}

func TestServerRuntimeInfo(t *testing.T) {
	configuration := &serverconfig.Configuration{
		EnableTCP: true,
		TCPSettings: settings.Settings{
			Network: settings.Network{
				Server: settings.Host{IPv4: "192.0.2.1"},
				Port:   8080,
			},
			Protocol: settings.TCP,
		},
	}
	info := serverRuntimeInfo(configuration)
	if len(info.Endpoints) != 1 || info.Endpoints[0].Protocol != settings.TCP {
		t.Fatalf("runtime info = %+v", info)
	}
}

func TestEndpointInfo(t *testing.T) {
	if _, ok := endpointInfo(settings.TCP, settings.Settings{}); ok {
		t.Fatal("endpointInfo() returned an empty endpoint")
	}

	protocolSettings := settings.Settings{
		Network:  settings.Network{IPv4: netip.MustParseAddr("10.0.0.1")},
		Protocol: settings.WS,
	}
	endpoint, ok := endpointInfo(settings.TCP, protocolSettings)
	if !ok || endpoint.Protocol != settings.WS || endpoint.TunnelIPv4 != protocolSettings.IPv4 {
		t.Fatalf("endpointInfo() = %+v, %v", endpoint, ok)
	}
}

func validClientConfiguration() *clientconfig.Configuration {
	return &clientconfig.Configuration{
		ClientID: 1,
		Protocol: settings.TCP,
		TCPSettings: settings.Settings{
			Network: settings.Network{
				TunName:    "tun0",
				IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24"),
				Server:     settings.Host{IPv4: "192.0.2.1"},
				Port:       8080,
			},
			MTU:      1500,
			Protocol: settings.TCP,
		},
		X25519PublicKey:  make([]byte, 32),
		ClientPublicKey:  make([]byte, 32),
		ClientPrivateKey: make([]byte, 32),
	}
}

func newTestTUI(t *testing.T) *TUI {
	t.Helper()
	directory := t.TempDir()
	ui, err := New(
		clientconfig.Files(),
		serverconfig.NewFile(filepath.Join(directory, "server_configuration.json")),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	return ui
}

func requireNoTTY(t *testing.T) {
	t.Helper()
	in, out, err := tea.OpenTTY()
	if err != nil {
		return
	}
	_ = in.Close()
	_ = out.Close()
	t.Skip("test requires a headless process without a controlling TTY")
}
