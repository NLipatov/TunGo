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
	isInteractiveTerminal() bool
	enableRuntimeLogCapture(capacity int)
	disableRuntimeLogCapture()
	runRuntimeDashboard(ctx context.Context, mode RuntimeMode) (bool, error)
}

var activeRuntimeBackend runtimeBackend = newBubbleTeaRuntimeBackend()

func IsInteractiveRuntime() bool {
	return activeRuntimeBackend.isInteractiveTerminal()
}

func EnableRuntimeLogCapture(capacity int) {
	activeRuntimeBackend.enableRuntimeLogCapture(capacity)
}

func DisableRuntimeLogCapture() {
	activeRuntimeBackend.disableRuntimeLogCapture()
}

func RunRuntimeDashboard(ctx context.Context, mode RuntimeMode) (bool, error) {
	return activeRuntimeBackend.runRuntimeDashboard(ctx, mode)
}
