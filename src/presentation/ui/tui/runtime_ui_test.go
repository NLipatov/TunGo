package tui

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"tungo/infrastructure/settings"
	runnerCommon "tungo/runtime"
)

type runtimeUITestBackend struct {
	enabledCap int
	disabled   bool
	run        func(ctx context.Context, mode RuntimeMode, options RuntimeUIOptions) (bool, error)
}

func (m *runtimeUITestBackend) enableRuntimeLogCapture(capacity int) { m.enabledCap = capacity }

func (m *runtimeUITestBackend) disableRuntimeLogCapture() { m.disabled = true }

func (m *runtimeUITestBackend) runRuntimeDashboard(ctx context.Context, mode RuntimeMode, options RuntimeUIOptions) (bool, error) {
	if m.run == nil {
		return false, nil
	}
	return m.run(ctx, mode, options)
}

func TestRuntimeUI_Wrappers(t *testing.T) {
	prevBackend := activeRuntimeBackend
	t.Cleanup(func() {
		activeRuntimeBackend = prevBackend
	})

	mock := &runtimeUITestBackend{}
	activeRuntimeBackend = mock

	EnableRuntimeLogCapture(42)
	if mock.enabledCap != 42 {
		t.Fatalf("expected capture capacity 42, got %d", mock.enabledCap)
	}

	DisableRuntimeLogCapture()
	if !mock.disabled {
		t.Fatal("expected disable wrapper to call implementation")
	}

	mock.run = func(_ context.Context, mode RuntimeMode, options RuntimeUIOptions) (bool, error) {
		if mode != RuntimeModeServer {
			t.Fatalf("expected server mode mapping, got %q", mode)
		}
		if len(options.Endpoints) != 1 {
			t.Fatalf("expected one endpoint entry, got %d", len(options.Endpoints))
		}
		if ipv4, ok := options.Endpoints[0].Server.IPv4(); !ok || ipv4 != netip.MustParseAddr("198.51.100.1") {
			t.Fatalf("expected forwarded server IPv4, got %v ok=%v", ipv4, ok)
		}
		if options.Protocol != settings.UDP {
			t.Fatalf("expected forwarded protocol UDP, got %v", options.Protocol)
		}
		return true, nil
	}
	quit, err := RunRuntimeDashboard(context.Background(), RuntimeModeServer, RuntimeUIOptions{
		Endpoints: []runnerCommon.EndpointInfo{{
			Protocol: settings.UDP,
			Server:   settings.Host{}.WithIPv4(netip.MustParseAddr("198.51.100.1")),
		}},
		Protocol: settings.UDP,
	})
	if err != nil || !quit {
		t.Fatalf("expected quit=true nil err, got quit=%v err=%v", quit, err)
	}

	mock.run = func(_ context.Context, mode RuntimeMode, _ RuntimeUIOptions) (bool, error) {
		if mode != RuntimeModeClient {
			t.Fatalf("expected client mode mapping, got %q", mode)
		}
		return false, errors.New("boom")
	}
	quit, err = RunRuntimeDashboard(context.Background(), RuntimeModeClient, RuntimeUIOptions{})
	if err == nil || quit {
		t.Fatalf("expected propagated error and quit=false, got quit=%v err=%v", quit, err)
	}

	mock.run = func(_ context.Context, mode RuntimeMode, _ RuntimeUIOptions) (bool, error) {
		if mode != RuntimeModeClient {
			t.Fatalf("expected client mode mapping, got %q", mode)
		}
		return false, ErrUserExit
	}
	quit, err = RunRuntimeDashboard(context.Background(), RuntimeModeClient, RuntimeUIOptions{})
	if !errors.Is(err, ErrUserExit) || quit {
		t.Fatalf("expected ErrUserExit and quit=false, got quit=%v err=%v", quit, err)
	}
}
