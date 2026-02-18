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

var (
	bubbleIsInteractiveRuntime = bubbleTea.IsInteractiveTerminal
	bubbleEnableLogCapture     = bubbleTea.EnableGlobalRuntimeLogCapture
	bubbleDisableLogCapture    = bubbleTea.DisableGlobalRuntimeLogCapture
	bubbleRunRuntimeDashboard  = bubbleTea.RunRuntimeDashboard
	bubbleRuntimeLogFeed       = bubbleTea.GlobalRuntimeLogFeed
)

func IsInteractiveRuntime() bool {
	return bubbleIsInteractiveRuntime()
}

func EnableRuntimeLogCapture(capacity int) {
	bubbleEnableLogCapture(capacity)
}

func DisableRuntimeLogCapture() {
	bubbleDisableLogCapture()
}

func RunRuntimeDashboard(ctx context.Context, mode RuntimeMode) (bool, error) {
	options := bubbleTea.RuntimeDashboardOptions{
		Mode:    bubbleTea.RuntimeDashboardClient,
		LogFeed: bubbleRuntimeLogFeed(),
	}
	if mode == RuntimeModeServer {
		options.Mode = bubbleTea.RuntimeDashboardServer
	}
	return bubbleRunRuntimeDashboard(ctx, options)
}
