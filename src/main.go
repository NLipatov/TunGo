package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"tungo/domain/app"
	"tungo/domain/mode"
	"tungo/infrastructure/PAL"
	"tungo/infrastructure/PAL/configuration/client"
	serverConf "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/PAL/linux/network_tools/netfilter"
	"tungo/infrastructure/PAL/stat"
	"tungo/infrastructure/PAL/tun_server"
	"tungo/infrastructure/routing/client_routing/client_factory"
	"tungo/presentation/configuring"
	"tungo/presentation/elevation"
	"tungo/presentation/interactive_commands/handlers"
	clientConf "tungo/presentation/runners/client"
	"tungo/presentation/runners/server"
	"tungo/presentation/runners/version"
)

func main() {
	processElevation := elevation.NewProcessElevation()
	if !processElevation.IsElevated() {
		fmt.Printf("Warning: %s must be run with admin privileges", app.Name)
		return
	}

	appCtx, appCtxCancel := context.WithCancel(context.Background())
	defer appCtxCancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-sigChan
		fmt.Println("\nInterrupt received. Shutting down...")
		appCtxCancel()
	}()

	configurationManager, configurationManagerErr := serverConf.NewManager(
		serverConf.NewServerResolver(),
		stat.NewDefaultStat(),
	)
	if configurationManagerErr != nil {
		log.Fatalf("could not instantiate server configuration manager: %s", configurationManagerErr)
	}

	configuratorFactory := configuring.NewConfigurationFactory(configurationManager)
	configurator := configuratorFactory.Configurator()
	appMode, appModeErr := configurator.Configure()
	if appModeErr != nil {
		log.Printf("%v", appModeErr)
		os.Exit(1)
	}

	switch appMode {
	case mode.Server:
		fmt.Printf("Starting server...\n")
		startServer(appCtx, configurationManager)
	case mode.ServerConfGen:
		err := generateClientConfiguration(configurationManager)
		if err != nil {
			log.Fatalf("failed to generate client configuration: %s", err)
		}
		os.Exit(0)
	case mode.Client:
		fmt.Printf("Starting client...\n")
		startClient(appCtx)
	case mode.Version:
		printVersion(appCtx)
	default:
		log.Printf("invalid app mode: %v", appMode)
		os.Exit(1)
	}
}

func generateClientConfiguration(configurationManager serverConf.ServerConfigurationManager) error {
	configuration, configurationErr := configurationManager.Configuration()
	if configurationErr != nil {
		return configurationErr
	}

	keyManager := serverConf.NewEd25519KeyManager(configuration, configurationManager)
	keyErr := keyManager.PrepareKeys()
	if keyErr != nil {
		return keyErr
	}

	handler := handlers.NewConfgenHandler(configurationManager)
	handlerErr := handler.GenerateNewClientConf()
	if handlerErr != nil {
		return handlerErr
	}

	return nil
}

func startClient(appCtx context.Context) {
	deps := clientConf.NewDependencies(client.NewManager())
	depsErr := deps.Initialize()
	if depsErr != nil {
		log.Fatalf("init error: %s", depsErr)
	}

	routerFactory := client_factory.NewRouterFactory()

	runner := clientConf.NewRunner(deps, routerFactory)
	runner.Run(appCtx)
}

func startServer(appCtx context.Context, configurationManager serverConf.ServerConfigurationManager) {
	nf, _, nfErr := netfilter.NewAutoNetfilter(PAL.NewExecCommander())
	if nfErr != nil {
		log.Fatalf("failed to initialize netfilter: %v", nfErr)
	}

	tunFactory := tun_server.NewServerTunFactory(nf)

	conf, confErr := configurationManager.Configuration()
	if confErr != nil {
		log.Fatal(confErr)
	}

	deps := server.NewDependencies(
		tunFactory,
		*conf,
		serverConf.NewEd25519KeyManager(conf, configurationManager),
		configurationManager,
	)

	runner := server.NewRunner(deps)
	runner.Run(appCtx)
}

func printVersion(appCtx context.Context) {
	runner := version.NewRunner()
	runner.Run(appCtx)
}
