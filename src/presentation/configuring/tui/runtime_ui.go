package tui

import (
	"context"
	bubbleTea "tungo/presentation/configuring/tui/components/implementations/bubble_tea"
)

type RuntimeMode string

const (
	RuntimeModeClient RuntimeMode = "client"
	RuntimeModeServer RuntimeMode = "server"
)

func IsInteractiveRuntime() bool {
	return bubbleTea.IsInteractiveTerminal()
}

func EnableRuntimeLogCapture(capacity int) {
	bubbleTea.EnableGlobalRuntimeLogCapture(capacity)
}

func DisableRuntimeLogCapture() {
	bubbleTea.DisableGlobalRuntimeLogCapture()
}

func RunRuntimeDashboard(ctx context.Context, mode RuntimeMode) (bool, error) {
	options := bubbleTea.RuntimeDashboardOptions{
		Mode:    bubbleTea.RuntimeDashboardClient,
		LogFeed: bubbleTea.GlobalRuntimeLogFeed(),
	}
	if mode == RuntimeModeServer {
		options.Mode = bubbleTea.RuntimeDashboardServer
	}
	return bubbleTea.RunRuntimeDashboard(ctx, options)
}
