package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"tungo/application/configuration"
	"tungo/application/network/connection"
	"tungo/application/network/routing/tun"
	tunnelClient "tungo/infrastructure/PAL/tunnel/client"
	"tungo/infrastructure/tunnel/sessionplane/client_factory"
)

type clientRuntime struct {
	connectionFactory connection.Factory
	workerFactory     connection.ClientWorkerFactory
	tunManager        tun.ClientManager
	routerFactory     connection.TrafficRouterFactory
	ready             atomic.Bool
}

func newClient() (*clientRuntime, error) {
	control := configuration.NewDefaultClientControl()
	slog.Info("starting client")

	conf, err := control.ClientRuntimeConfiguration()
	if err != nil {
		return nil, fmt.Errorf("init error: failed to read client configuration: %w", err)
	}
	tunManager, err := tunnelClient.NewPlatformTunManager(conf)
	if err != nil {
		return nil, fmt.Errorf("init error: failed to configure tun: %w", err)
	}

	return &clientRuntime{
		connectionFactory: client_factory.NewConnectionFactory(conf),
		workerFactory:     client_factory.NewWorkerFactory(conf),
		tunManager:        tunManager,
		routerFactory:     client_factory.NewRouterFactory(),
	}, nil
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
		if err := r.tunManager.DisposeDevices(); err != nil {
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

	if err := r.tunManager.DisposeDevices(); err != nil {
		slog.Warn("failed to dispose TUN devices", "err", err)
	}

	router, conn, tun, err := r.routerFactory.
		CreateRouter(ctx, r.connectionFactory, r.tunManager, r.workerFactory)
	if err != nil {
		return fmt.Errorf("failed to create router: %w", err)
	}
	r.ready.Store(true)

	defer func() {
		_ = conn.Close()
		_ = tun.Close()
	}()

	slog.Info("tunneling traffic via TUN device")
	return router.RouteTraffic(ctx)
}

func (r *clientRuntime) Ready() bool {
	return r.ready.Load()
}
