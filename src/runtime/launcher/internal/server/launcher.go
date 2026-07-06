package server

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"tungo/infrastructure/PAL/configuration"
	serverConf "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/PAL/stat"
	tunnelServer "tungo/infrastructure/PAL/tunnel/server"
	"tungo/infrastructure/logging"
	"tungo/infrastructure/settings"
	appRuntime "tungo/runtime"
	runtimeServer "tungo/runtime/server"
)

func newDefaultConfiguration() (configuration.Resolver, serverConf.ConfigurationManager, error) {
	resolver := serverConf.NewServerResolver()
	manager, err := serverConf.NewManager(resolver, stat.NewDefaultStat())
	if err != nil {
		return nil, nil, fmt.Errorf("configuration error: %w", err)
	}
	return resolver, manager, nil
}

func Run(ctx context.Context) error {
	resolver, manager, err := newDefaultConfiguration()
	if err != nil {
		return err
	}
	return run(ctx, resolver, manager)
}

func Start(ctx context.Context) (appRuntime.Session, error) {
	resolver, manager, err := newDefaultConfiguration()
	if err != nil {
		return nil, err
	}
	return start(ctx, resolver, manager)
}

func run(
	ctx context.Context,
	resolver configuration.Resolver,
	manager serverConf.ConfigurationManager,
) error {
	runner, _, stopConfigWatcher, err := newRuntime(ctx, resolver, manager)
	if err != nil {
		return err
	}
	defer stopConfigWatcher()
	return runner.Run(ctx)
}

func start(
	ctx context.Context,
	resolver configuration.Resolver,
	manager serverConf.ConfigurationManager,
) (appRuntime.Session, error) {
	runner, conf, stopConfigWatcher, err := newRuntime(ctx, resolver, manager)
	if err != nil {
		return nil, err
	}

	sessionCtx, cancel := context.WithCancel(ctx)
	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			cancel()
			stopConfigWatcher()
		})
	}

	session := appRuntime.NewRunningSession(
		appRuntime.Info{
			Mode:      appRuntime.ModeServer,
			Endpoints: endpointsFromConfiguration(conf),
		},
		closedReadyCh(),
		stop,
	)

	go func() {
		defer stop()
		session.Finish(runner.Run(sessionCtx))
	}()
	return session, nil
}

func newRuntime(
	ctx context.Context,
	resolver configuration.Resolver,
	manager serverConf.ConfigurationManager,
) (*runtimeServer.Runner, serverConf.Configuration, context.CancelFunc, error) {
	configPath := setupCrashLog(resolver)
	if err := prepareKeys(manager); err != nil {
		return nil, serverConf.Configuration{}, nil, fmt.Errorf("key preparation failed: %w", err)
	}
	slog.Info("starting server", "config_path", configPath)

	tunFactory := tunnelServer.NewTunFactory()

	conf, confErr := manager.Configuration()
	if confErr != nil {
		return nil, serverConf.Configuration{}, nil, fmt.Errorf("failed to load server configuration: %w", confErr)
	}

	deps := runtimeServer.NewDependencies(
		tunFactory,
		*conf,
		serverConf.NewX25519KeyManager(manager),
		manager,
	)

	serverRuntime, err := tunnelServer.NewRuntime(manager)
	if err != nil {
		return nil, serverConf.Configuration{}, nil, fmt.Errorf("failed to create server runtime: %w", err)
	}

	workerFactory, err := tunnelServer.NewWorkerFactory(serverRuntime, manager)
	if err != nil {
		return nil, serverConf.Configuration{}, nil, fmt.Errorf("failed to create worker factory: %w", err)
	}

	configWatcher := serverConf.NewConfigWatcher(
		manager,
		serverRuntime.SessionRevoker(),
		serverRuntime.AllowedPeersUpdater(),
		configPath,
		serverConf.DefaultWatchInterval,
		logging.NewStdLogger(slog.LevelInfo),
	)
	watchCtx, watchCancel := context.WithCancel(ctx)
	go configWatcher.Watch(watchCtx)

	runner := runtimeServer.NewRunner(
		deps,
		workerFactory,
		tunnelServer.NewTrafficRouterFactory(),
	)
	return runner, *conf, watchCancel, nil
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

func closedReadyCh() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

type protocolSettings struct {
	protocol settings.Protocol
	settings settings.Settings
}

func endpointsFromConfiguration(conf serverConf.Configuration) []appRuntime.EndpointInfo {
	endpoints := make([]appRuntime.EndpointInfo, 0, 3)
	for _, enabledSetting := range enabledProtocolSettings(conf) {
		if endpoint, ok := appRuntime.EndpointInfoFromSettings(enabledSetting.protocol, enabledSetting.settings); ok {
			endpoints = append(endpoints, endpoint)
		}
	}
	return endpoints
}

func enabledProtocolSettings(conf serverConf.Configuration) []protocolSettings {
	result := make([]protocolSettings, 0, 3)
	if conf.EnableTCP {
		result = append(result, protocolSettings{protocol: settings.TCP, settings: conf.TCPSettings})
	}
	if conf.EnableUDP {
		result = append(result, protocolSettings{protocol: settings.UDP, settings: conf.UDPSettings})
	}
	if conf.EnableWS {
		result = append(result, protocolSettings{protocol: settings.WS, settings: conf.WSSettings})
	}
	return result
}
