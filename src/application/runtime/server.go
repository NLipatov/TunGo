package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"

	"tungo/application/configuration"
	platformServer "tungo/infrastructure/PAL/tunnel/server"
	tunnelServer "tungo/infrastructure/tunnel/server"
)

type serverRuntime struct {
	runServer func(context.Context, func()) error
	ready     atomic.Bool
	control   configuration.ServerRuntimeControl
	revoker   configuration.ServerSessionRevoker
	updater   configuration.ServerAllowedPeersUpdater
}

func newServer() (*serverRuntime, error) {
	control := configuration.NewServerControl()
	if control == nil {
		return nil, fmt.Errorf("server runtime is not supported on this platform")
	}

	slog.Info("starting server")

	tunFactory := platformServer.NewTunFactory()

	conf, confErr := control.ServerRuntimeConfiguration()
	if confErr != nil {
		return nil, fmt.Errorf("failed to load server configuration: %w", confErr)
	}

	tunnelRuntime, err := tunnelServer.New(conf, tunFactory)
	if err != nil {
		return nil, fmt.Errorf("failed to create server runtime: %w", err)
	}

	return &serverRuntime{
		runServer: tunnelRuntime.Run,
		control:   control,
		revoker:   tunnelRuntime,
		updater:   tunnelRuntime,
	}, nil
}

func (r *serverRuntime) Run(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		r.control.WatchServerRuntimeConfiguration(runCtx, r.revoker, r.updater)
	}()

	err := r.runServer(runCtx, func() { r.ready.Store(true) })
	cancel()
	<-watcherDone
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func (r *serverRuntime) Ready() bool {
	return r.ready.Load()
}
