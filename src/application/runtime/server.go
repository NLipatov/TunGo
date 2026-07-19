package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"tungo/application/configuration"
	"tungo/application/network/connection"
	"tungo/application/network/routing"
	"tungo/application/network/routing/tun"
	tunnelServer "tungo/infrastructure/PAL/tunnel/server"
	"tungo/infrastructure/settings"

	"golang.org/x/sync/errgroup"
)

type serverRuntime struct {
	config        configuration.ServerRuntimeConfiguration
	tunManager    tun.ServerManager
	workerFactory connection.ServerWorkerFactory
	routerFactory connection.ServerTrafficRouterFactory
	ready         *readySignal
	control       configuration.ServerRuntimeControl
	revoker       configuration.ServerSessionRevoker
	updater       configuration.ServerAllowedPeersUpdater
}

func newServer() (*serverRuntime, error) {
	control, err := configuration.NewDefaultServerControl()
	if err != nil {
		return nil, err
	}
	if control == nil {
		return nil, fmt.Errorf("server runtime is not supported on this platform")
	}

	slog.Info("starting server")

	tunFactory := tunnelServer.NewTunFactory()

	conf, confErr := control.ServerRuntimeConfiguration()
	if confErr != nil {
		return nil, fmt.Errorf("failed to load server configuration: %w", confErr)
	}

	tunnelRuntime, err := tunnelServer.NewRuntime(conf)
	if err != nil {
		return nil, fmt.Errorf("failed to create server runtime: %w", err)
	}

	workerFactory, err := tunnelServer.NewWorkerFactory(tunnelRuntime, conf)
	if err != nil {
		return nil, fmt.Errorf("failed to create worker factory: %w", err)
	}

	return &serverRuntime{
		config:        conf,
		tunManager:    tunFactory,
		workerFactory: workerFactory,
		routerFactory: tunnelServer.NewTrafficRouterFactory(),
		ready:         newReadySignal(),
		control:       control,
		revoker:       tunnelRuntime.SessionRevoker(),
		updater:       tunnelRuntime.AllowedPeersUpdater(),
	}, nil
}

func (r *serverRuntime) Run(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		r.control.WatchServerRuntimeConfiguration(runCtx, r.revoker, r.updater)
	}()

	err := r.run(runCtx)
	cancel()
	<-watcherDone
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func (r *serverRuntime) run(ctx context.Context) error {
	if err := r.cleanup(); err != nil {
		slog.Warn("preflight cleanup error", "err", err)
	}
	defer func() {
		if err := r.cleanup(); err != nil {
			slog.Warn("postflight cleanup error", "err", err)
		}
	}()

	return r.runWorkers(ctx)
}

func (r *serverRuntime) cleanup() error {
	var group errgroup.Group
	for _, workerSettings := range r.config.AllSettings() {
		group.Go(func() error {
			return r.tunManager.DisposeDevices(workerSettings)
		})
	}
	return group.Wait()
}

func (r *serverRuntime) runWorkers(ctx context.Context) error {
	workersCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	group, groupCtx := errgroup.WithContext(workersCtx)
	for _, workerSettings := range r.config.EnabledSettings() {
		router, err := r.createRouter(groupCtx, workerSettings)
		if err != nil {
			cancel()
			_ = group.Wait()
			return fmt.Errorf("could not create %s router: %w", workerSettings.Protocol, err)
		}
		protocol := workerSettings.Protocol
		group.Go(func() error {
			if err := router.RouteTraffic(groupCtx); err != nil {
				return fmt.Errorf("%s worker failed: %w", protocol, err)
			}
			return nil
		})
	}
	r.ready.mark()
	return group.Wait()
}

func (r *serverRuntime) createRouter(
	ctx context.Context,
	workerSettings settings.Settings,
) (routing.Router, error) {
	tunDevice, err := r.tunManager.CreateDevice(workerSettings)
	if err != nil {
		return nil, fmt.Errorf("error creating tun device: %w", err)
	}
	worker, err := r.workerFactory.CreateWorker(ctx, tunDevice, workerSettings)
	if err != nil {
		_ = tunDevice.Close()
		return nil, fmt.Errorf("error creating worker: %w", err)
	}
	return r.routerFactory.CreateRouter(worker), nil
}

func (r *serverRuntime) WaitForReady(ctx context.Context) error {
	return r.ready.wait(ctx)
}
