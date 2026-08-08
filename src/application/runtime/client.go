package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"tungo/application/configuration"
	tunnelClient "tungo/infrastructure/PAL/tunnel/client"
	"tungo/infrastructure/tunnel/client"
)

type clientRuntime struct {
	runSession     func(context.Context, func()) error
	disposeDevices func() error
	ready          atomic.Bool
}

func newClient() (*clientRuntime, error) {
	control := configuration.NewClientControl()
	slog.Info("starting client")

	conf, err := control.ClientRuntimeConfiguration()
	if err != nil {
		return nil, fmt.Errorf("init error: failed to read client configuration: %w", err)
	}
	tunManager, err := tunnelClient.NewPlatformTunManager(conf.Settings, conf.CleanupSettings)
	if err != nil {
		return nil, fmt.Errorf("init error: failed to configure tun: %w", err)
	}

	tunnel := client.New(conf, tunManager)
	return &clientRuntime{runSession: tunnel.Run, disposeDevices: tunManager.DisposeDevices}, nil
}

func (r *clientRuntime) Run(ctx context.Context) error {
	err := r.run(ctx)
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func (r *clientRuntime) run(ctx context.Context) error {
	defer func() {
		if err := r.disposeDevices(); err != nil {
			slog.Warn("failed to dispose TUN devices on exit", "err", err)
		}
	}()

	for ctx.Err() == nil {
		err := r.runAttempt(ctx)
		switch {
		case err == nil:
			return nil
		case errors.Is(err, context.Canceled):
			return context.Canceled
		default:
			slog.Warn("session error, reconnecting", "err", err)
			timer := time.NewTimer(500 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return context.Canceled
			case <-timer.C:
			}
		}
	}
	return context.Canceled
}

func (r *clientRuntime) runAttempt(parentCtx context.Context) error {
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	if err := r.disposeDevices(); err != nil {
		slog.Warn("failed to dispose TUN devices", "err", err)
	}

	return r.runSession(ctx, func() { r.ready.Store(true) })
}

func (r *clientRuntime) Ready() bool {
	return r.ready.Load()
}
