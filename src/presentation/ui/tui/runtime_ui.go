package tui

import (
	"context"
	"net/netip"
)

type RuntimeMode string

const (
	RuntimeModeClient RuntimeMode = "client"
	RuntimeModeServer RuntimeMode = "server"
)

type RuntimeAddressInfo struct {
	ServerIPv4  netip.Addr
	ServerIPv6  netip.Addr
	NetworkIPv4 netip.Addr
	NetworkIPv6 netip.Addr
}

type RuntimeUIOptions struct {
	ReadyCh <-chan struct{}
	Address RuntimeAddressInfo
}

type runtimeBackend interface {
	enableRuntimeLogCapture(capacity int)
	disableRuntimeLogCapture()
	runRuntimeDashboard(ctx context.Context, mode RuntimeMode, options RuntimeUIOptions) (bool, error)
}

var activeRuntimeBackend runtimeBackend = newBubbleTeaRuntimeBackend()

func EnableRuntimeLogCapture(capacity int) {
	activeRuntimeBackend.enableRuntimeLogCapture(capacity)
}

func DisableRuntimeLogCapture() {
	activeRuntimeBackend.disableRuntimeLogCapture()
}

func RunRuntimeDashboard(ctx context.Context, mode RuntimeMode, options RuntimeUIOptions) (bool, error) {
	return activeRuntimeBackend.runRuntimeDashboard(ctx, mode, options)
}
