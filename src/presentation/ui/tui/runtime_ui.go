package tui

import (
	"context"
)

type RuntimeMode string

const (
	RuntimeModeClient RuntimeMode = "client"
	RuntimeModeServer RuntimeMode = "server"
)

type runtimeBackend interface {
	enableRuntimeLogCapture(capacity int)
	disableRuntimeLogCapture()
	runRuntimeDashboard(ctx context.Context, mode RuntimeMode) (bool, error)
}

var activeRuntimeBackend runtimeBackend = newBubbleTeaRuntimeBackend()

func EnableRuntimeLogCapture(capacity int) {
	activeRuntimeBackend.enableRuntimeLogCapture(capacity)
}

func DisableRuntimeLogCapture() {
	activeRuntimeBackend.disableRuntimeLogCapture()
}

func RunRuntimeDashboard(ctx context.Context, mode RuntimeMode) (bool, error) {
	return activeRuntimeBackend.runRuntimeDashboard(ctx, mode)
}
