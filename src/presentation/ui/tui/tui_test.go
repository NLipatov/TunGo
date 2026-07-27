package tui

import (
	"context"
	"errors"
	"testing"

	appConfiguration "tungo/application/configuration"
	"tungo/application/runtime"
	"tungo/infrastructure/settings"
	bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"
)

func TestNewTUI(t *testing.T) {
	ui, err := New(configurationControlsMock(true), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ui == nil {
		t.Fatal("expected non-nil TUI")
	}
	if ui.configuratorOptions.ClientConfigurationControl == nil {
		t.Fatal("expected client configuration control to be registered")
	}
	if ui.configuratorOptions.ServerConfigurationControl == nil {
		t.Fatal("expected server configuration control to be registered")
	}
}

func TestNewTUIRejectsMissingClientControl(t *testing.T) {
	ui, err := New(appConfiguration.Controls{}, nil)
	if err == nil || ui != nil {
		t.Fatalf("New() = %v, %v; want nil and error", ui, err)
	}
}

type runtimeInfoErrorControl struct {
	configurationControlMock
	err error
}

func (c runtimeInfoErrorControl) RuntimeInfo() (appConfiguration.RuntimeInfo, error) {
	return appConfiguration.RuntimeInfo{}, c.err
}

func TestTUI_Run_CanceledContext_ReturnsNil(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ui := newTestTUI(t)

	err := ui.Run(ctx)
	if err != nil {
		t.Fatalf("expected canceled context to stop run loop cleanly, got %v", err)
	}
}

func TestTUI_RunRuntime_RuntimeInfoError(t *testing.T) {
	want := errors.New("runtime info failed")
	ui := newTestTUI(t)
	ui.configuratorOptions.ClientConfigurationControl = runtimeInfoErrorControl{err: want}

	err := ui.runRuntime(
		context.Background(),
		runtime.ModeClient,
		bubbleTea.NewRuntimeLogBuffer(8),
	)
	if err == nil || err.Error() != "runtime info error: runtime info failed" {
		t.Fatalf("expected runtime info error, got %v", err)
	}
}

func TestTUI_RuntimeInfo_Client(t *testing.T) {
	ui := newTestTUI(t)

	got, err := ui.runtimeInfo(runtime.ModeClient)
	if err != nil {
		t.Fatalf("runtimeInfo() error = %v", err)
	}
	if got.Protocol != settings.TCP {
		t.Fatalf("expected client protocol TCP, got %v", got.Protocol)
	}
}

func TestTUI_RuntimeInfo_Server(t *testing.T) {
	ui := newTestTUI(t)

	got, err := ui.runtimeInfo(runtime.ModeServer)
	if err != nil {
		t.Fatalf("runtimeInfo() error = %v", err)
	}
	if got.Protocol != settings.TCP {
		t.Fatalf("expected server protocol TCP, got %v", got.Protocol)
	}
}

func TestTUI_RuntimeInfo_MissingClientControl(t *testing.T) {
	ui := newTestTUI(t)
	ui.configuratorOptions.ClientConfigurationControl = nil

	_, err := ui.runtimeInfo(runtime.ModeClient)
	if err == nil || err.Error() != "client configuration control is nil" {
		t.Fatalf("expected missing client control error, got %v", err)
	}
}

func TestTUI_RuntimeInfo_MissingServerControl(t *testing.T) {
	ui := newTestTUI(t)
	ui.configuratorOptions.ServerConfigurationControl = nil

	_, err := ui.runtimeInfo(runtime.ModeServer)
	if err == nil || err.Error() != "server configuration control is nil" {
		t.Fatalf("expected missing server control error, got %v", err)
	}
}

func TestTUI_RuntimeInfo_InvalidMode(t *testing.T) {
	ui := newTestTUI(t)

	_, err := ui.runtimeInfo(0)
	if err == nil || err.Error() != "invalid runtime mode: 0" {
		t.Fatalf("expected invalid runtime mode error, got %v", err)
	}
}

func newTestTUI(t *testing.T) *TUI {
	t.Helper()
	ui, err := New(configurationControlsMock(true), nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return ui
}

type configurationControlMock struct{}

func configurationControlsMock(serverSupported bool) appConfiguration.Controls {
	controls := appConfiguration.Controls{Client: configurationControlMock{}}
	if serverSupported {
		controls.Server = configurationControlMock{}
	}
	return controls
}

func (configurationControlMock) List() ([]string, error) {
	return nil, nil
}

func (configurationControlMock) Select(string) error {
	return nil
}

func (configurationControlMock) ValidateActive() error {
	return nil
}

func (configurationControlMock) RuntimeInfo() (appConfiguration.RuntimeInfo, error) {
	return appConfiguration.RuntimeInfo{Protocol: settings.TCP}, nil
}

func (configurationControlMock) CreateFromJSON(string, string) error {
	return nil
}

func (configurationControlMock) Delete(string) error {
	return nil
}

func (configurationControlMock) ClientRuntimeConfiguration() (appConfiguration.ClientRuntimeConfiguration, error) {
	return appConfiguration.ClientRuntimeConfiguration{}, nil
}

func (configurationControlMock) GenerateClientConfiguration() (appConfiguration.GeneratedClientConfiguration, error) {
	return appConfiguration.GeneratedClientConfiguration{}, nil
}

func (configurationControlMock) ListPeers() ([]appConfiguration.ServerPeer, error) {
	return nil, nil
}

func (configurationControlMock) SetPeerEnabled(int, bool) error {
	return nil
}

func (configurationControlMock) RemovePeer(int) error {
	return nil
}

func (configurationControlMock) ServerRuntimeConfiguration() (appConfiguration.ServerRuntimeConfiguration, error) {
	return appConfiguration.ServerRuntimeConfiguration{}, nil
}

func (configurationControlMock) WatchServerRuntimeConfiguration(
	context.Context,
	appConfiguration.ServerSessionRevoker,
	appConfiguration.ServerAllowedPeersUpdater,
) {
}
