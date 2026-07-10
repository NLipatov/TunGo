package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"tungo/infrastructure/PAL/configuration"
	serverConf "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/PAL/stat"
	tunnelServer "tungo/infrastructure/PAL/tunnel/server"
	"tungo/infrastructure/logging"
)

type Runtime struct {
	runner        *Runner
	configWatcher *serverConf.ConfigWatcher
}

func NewDefaultConfiguration() (configuration.Resolver, serverConf.ConfigurationManager, error) {
	resolver := serverConf.NewServerResolver()
	manager, err := serverConf.NewManager(resolver, stat.NewDefaultStat())
	if err != nil {
		return nil, nil, fmt.Errorf("configuration error: %w", err)
	}
	return resolver, manager, nil
}

func NewRuntime(
	resolver configuration.Resolver,
	manager serverConf.ConfigurationManager,
) (*Runtime, error) {
	configPath := setupCrashLog(resolver)
	if err := prepareKeys(manager); err != nil {
		return nil, fmt.Errorf("key preparation failed: %w", err)
	}
	slog.Info("starting server", "config_path", configPath)

	tunFactory := tunnelServer.NewTunFactory()

	conf, confErr := manager.Configuration()
	if confErr != nil {
		return nil, fmt.Errorf("failed to load server configuration: %w", confErr)
	}

	deps := NewDependencies(
		tunFactory,
		*conf,
		serverConf.NewX25519KeyManager(manager),
		manager,
	)

	serverRuntime, err := tunnelServer.NewRuntime(manager)
	if err != nil {
		return nil, fmt.Errorf("failed to create server runtime: %w", err)
	}

	workerFactory, err := tunnelServer.NewWorkerFactory(serverRuntime, manager)
	if err != nil {
		return nil, fmt.Errorf("failed to create worker factory: %w", err)
	}

	configWatcher := serverConf.NewConfigWatcher(
		manager,
		serverRuntime.SessionRevoker(),
		serverRuntime.AllowedPeersUpdater(),
		configPath,
		serverConf.DefaultWatchInterval,
		logging.NewStdLogger(slog.LevelInfo),
	)
	return &Runtime{
		runner: NewRunner(
			deps,
			workerFactory,
			tunnelServer.NewTrafficRouterFactory(),
		),
		configWatcher: configWatcher,
	}, nil
}

func (r *Runtime) Run(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	var watcherDone chan struct{}
	if r.configWatcher != nil {
		watcherDone = make(chan struct{})
		go func() {
			defer close(watcherDone)
			r.configWatcher.Watch(runCtx)
		}()
	}

	err := r.runner.Run(runCtx)
	cancel()
	if watcherDone != nil {
		<-watcherDone
	}
	if err == nil || ctx.Err() != nil ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	return err
}

func (r *Runtime) WaitForReady(ctx context.Context) error {
	return r.runner.WaitForReady(ctx)
}

func prepareKeys(manager serverConf.ConfigurationManager) error {
	keyManager := serverConf.NewX25519KeyManager(manager)
	if err := keyManager.PrepareKeys(); err != nil {
		return fmt.Errorf("could not prepare keys: %w", err)
	}
	return nil
}

func setupCrashLog(resolver configuration.Resolver) string {
	configPath, err := resolver.Resolve()
	if err != nil {
		return ""
	}
	logging.SetCrashOutput(filepath.Join(filepath.Dir(configPath), "crash.log"))
	return configPath
}
