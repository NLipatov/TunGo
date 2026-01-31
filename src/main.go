package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
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
	"tungo/presentation/interactive_commands/handlers"
	clientConf "tungo/presentation/runners/client"
	"tungo/presentation/runners/server"
	"tungo/presentation/runners/version"
	"tungo/presentation/signals/shutdown"
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

	configurationManager, configurationManagerErr := serverConf.NewManager(
		serverConf.NewServerResolver(),
		stat.NewDefaultStat(),
	)
	if configurationManagerErr != nil {
		log.Printf("could not instantiate server configuration manager: %s", configurationManagerErr)
		exitCode = 1
		return
	}
	keyManager := serverConf.NewEd25519KeyManager(configurationManager)
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
		err := startServer(appCtx, configurationManager)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return
			}
			log.Printf("Server finished with error: %v", err)
			exitCode = 2
			return
		}
	case mode.ServerConfGen:
		handler := handlers.NewConfgenHandler(
			configurationManager,
			handlers.NewJsonMarshaller(),
		)
		if err := handler.GenerateNewClientConf(); err != nil {
			log.Printf("failed to generate client configuration: %v", err)
			exitCode = 1
			return
		}
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
) error {
	tunFactory := tun_server.NewServerTunFactory()

	conf, confErr := configurationManager.Configuration()
	if confErr != nil {
		return fmt.Errorf("failed to load server configuration: %w", confErr)
	}

	deps := server.NewDependencies(
		tunFactory,
		*conf,
		serverConf.NewEd25519KeyManager(configurationManager),
		configurationManager,
	)

	runner := server.NewRunner(
		deps,
		tun_server.NewServerWorkerFactory(configurationManager),
		tun_server.NewServerTrafficRouterFactory(),
	)
	return runner.Run(ctx)
}

func printVersion(appCtx context.Context) {
	runner := version.NewRunner()
	runner.Run(appCtx)
}
