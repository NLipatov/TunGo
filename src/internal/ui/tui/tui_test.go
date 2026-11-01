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
	ui := New(
		serverconfig.NewFile(filepath.Join(directory, "server_configuration.json")),
		nil,
	)
	if ui.clientConfigurations == nil || ui.serverFile == nil {
		t.Fatal("configuration files were not registered")
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
	active := settings.Settings{
		Protocol: settings.TCP,
		Network: settings.Network{
			Server: settings.Host{IPv4: "192.0.2.1", IPv6: "2001:db8::1"},
			Port:   8080,
			IPv4:   netip.MustParseAddr("10.0.0.2"),
			IPv6:   netip.MustParseAddr("fd00::2"),
		},
	}
	info := clientRuntimeInfo(active)
	if info.Protocol != settings.TCP || len(info.Endpoints) != 1 || info.Endpoints[0].Port != 8080 ||
		info.Endpoints[0].Protocol != active.Protocol || info.Endpoints[0].Server != active.Server ||
		info.Endpoints[0].TunnelIPv4 != active.IPv4 || info.Endpoints[0].TunnelIPv6 != active.IPv6 {
		t.Fatalf("runtime info = %+v", info)
	}
}

func TestClientRuntimeInfoBeforeFirstConnectionAttempt(t *testing.T) {
	info := clientRuntimeInfo(settings.Settings{})
	if info.Protocol != settings.UNKNOWN || len(info.Endpoints) != 0 {
		t.Fatalf("runtime info before settings are loaded = %+v", info)
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

func newTestTUI(t *testing.T) *TUI {
	t.Helper()
	directory := t.TempDir()
	ui := New(
		serverconfig.NewFile(filepath.Join(directory, "server_configuration.json")),
		nil,
	)
	ui.clientConfigurations = clientconfig.NewConfigurations(directory)
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
