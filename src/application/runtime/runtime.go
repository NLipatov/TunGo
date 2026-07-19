package runtime

import (
	"context"
	"fmt"
	"path/filepath"

	"tungo/application/configuration"
	"tungo/infrastructure/logging"
)

type Mode uint8

const (
	ModeClient Mode = iota + 1
	ModeServer
)

// Runtime is a single-use runtime instance.
type Runtime interface {
	// Run blocks until the runtime stops. Context cancellation is a clean stop;
	// operational failures are returned as errors.
	Run(context.Context) error
	// WaitForReady blocks until a concurrent Run call makes the runtime
	// ready to serve traffic, or until ctx ends. It does not start the runtime.
	WaitForReady(context.Context) error
}

func New(mode Mode) (Runtime, error) {
	switch mode {
	case ModeServer:
		setupCrashLog()
		return newServer()
	case ModeClient:
		setupCrashLog()
		return newClient()
	default:
		return nil, fmt.Errorf("invalid runtime mode: %v", mode)
	}
}

func setupCrashLog() {
	directory, err := configuration.DefaultStorageDirectory()
	if err == nil && directory != "" {
		logging.SetCrashOutput(filepath.Join(directory, "crash.log"))
	}
}
