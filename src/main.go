package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"
	"tungo/domain/app"
	"tungo/domain/mode"
	"tungo/infrastructure/PAL/configuration/client"
	serverConf "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/PAL/signal"
	"tungo/infrastructure/PAL/stat"
	"tungo/infrastructure/PAL/tun_server"
	"tungo/infrastructure/tunnel/sessionplane/client_factory"
	"tungo/presentation/configuring"
	"tungo/presentation/elevation"
	"encoding/json"
	"tungo/application/confgen"
	"tungo/infrastructure/cryptography/primitives"
	clientConf "tungo/presentation/runners/client"
	"tungo/presentation/runners/server"
	"tungo/presentation/runners/version"
	"tungo/presentation/signals/shutdown"
)

const (
	// configWatchInterval is how often the server checks for AllowedPeers changes.
	// Sessions for removed/disabled peers are revoked on detection.
	configWatchInterval = 30 * time.Second
)

func main() {
	exitCode := 0
	appCtx, appCtxCancel := context.WithCancel(context.Background())
	defer func() {
		os.Exit(exitCode)
	}()
	defer appCtxCancel()
	// handle shutdown signals
	shutdownSignalHandler := shutdown.NewHandler(
		appCtx,
		appCtxCancel,
		signal.NewDefaultProvider(),
		shutdown.NewNotifier(),
	)
	shutdownSignalHandler.Handle()

	processElevation := elevation.NewProcessElevation()
	if !processElevation.IsElevated() {
		log.Printf("%s must be run with admin privileges", app.Name)
		exitCode = 1
		return
	}

	serverResolver := serverConf.NewServerResolver()
	configurationManager, configurationManagerErr := serverConf.NewManager(
		serverResolver,
		stat.NewDefaultStat(),
	)
	if configurationManagerErr != nil {
		log.Printf("could not instantiate server configuration manager: %s", configurationManagerErr)
		exitCode = 1
		return
	}
	serverConfigPath, _ := serverResolver.Resolve()
	keyManager := serverConf.NewX25519KeyManager(configurationManager)
	if pKeysErr := keyManager.PrepareKeys(); pKeysErr != nil {
		log.Printf("could not prepare keys: %s", pKeysErr)
		exitCode = 1
		return
	}

	configuratorFactory := configuring.NewConfigurationFactory(configurationManager)
	configurator := configuratorFactory.Configurator()
	appMode, appModeErr := configurator.Configure()
	if appModeErr != nil {
		log.Printf("%v", appModeErr)
		exitCode = 1
		return
	}

	switch appMode {
	case mode.Server:
		log.Printf("Starting server...")
		err := startServer(appCtx, configurationManager, serverConfigPath)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return
			}
			log.Printf("Server finished with error: %v", err)
			exitCode = 2
			return
		}
	case mode.ServerConfGen:
		gen := confgen.NewGenerator(configurationManager, &primitives.DefaultKeyDeriver{})
		conf, err := gen.Generate()
		if err != nil {
			log.Printf("failed to generate client configuration: %v", err)
			exitCode = 1
			return
		}
		data, err := json.MarshalIndent(conf, "", "  ")
		if err != nil {
			log.Printf("failed to marshal client configuration: %v", err)
			exitCode = 1
			return
		}
		fmt.Println(string(data))
	case mode.Client:
		log.Printf("Starting client...")
		if err := startClient(appCtx); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return
			}
			log.Printf("Client finished with error: %v", err)
			exitCode = 2
			return
		}
	case mode.Version:
		printVersion(appCtx)
	default:
		log.Printf("invalid app mode: %v", appMode)
		exitCode = 1
		return
	}
}

func startClient(appCtx context.Context) error {
	deps := clientConf.NewDependencies(client.NewManager())
	depsErr := deps.Initialize()
	if depsErr != nil {
		return fmt.Errorf("init error: %w", depsErr)
	}

	routerFactory := client_factory.NewRouterFactory()

	runner := clientConf.NewRunner(deps, routerFactory)
	runner.Run(appCtx)
	return nil
}

func startServer(
	ctx context.Context,
	configurationManager serverConf.ConfigurationManager,
	configPath string,
) error {
	tunFactory := tun_server.NewServerTunFactory()

	conf, confErr := configurationManager.Configuration()
	if confErr != nil {
		return fmt.Errorf("failed to load server configuration: %w", confErr)
	}

	deps := server.NewDependencies(
		tunFactory,
		*conf,
		serverConf.NewX25519KeyManager(configurationManager),
		configurationManager,
	)

	workerFactory, err := tun_server.NewServerWorkerFactory(configurationManager)
	if err != nil {
		return fmt.Errorf("failed to create worker factory: %w", err)
	}

	// Start ConfigWatcher to revoke sessions and update AllowedPeers at runtime
	configWatcher := serverConf.NewConfigWatcher(
		configurationManager,
		workerFactory.SessionRevoker(),
		workerFactory.AllowedPeersUpdater(),
		configPath,
		configWatchInterval,
		log.Default(),
	)
	go configWatcher.Watch(ctx)

	runner := server.NewRunner(
		deps,
		workerFactory,
		tun_server.NewServerTrafficRouterFactory(),
	)
	return runner.Run(ctx)
}

func printVersion(appCtx context.Context) {
	runner := version.NewRunner()
	runner.Run(appCtx)
}
