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
	"tungo/infrastructure/PAL/pal_factory"
	"tungo/infrastructure/routing/client_routing/client_factory"
	"tungo/presentation/configuring"
	"tungo/presentation/elevation"
	"tungo/presentation/interactive_commands/handlers"
	"tungo/presentation/runners/client"
	"tungo/presentation/runners/server"
	"tungo/presentation/runners/version"
	"tungo/settings/client_configuration"
	"tungo/settings/server_configuration"
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

	configuratorFactory := configuring.NewConfigurationFactory()
	configurator := configuratorFactory.Configurator()
	appMode, appModeErr := configurator.Configure()
	if appModeErr != nil {
		log.Printf("%v", appModeErr)
		os.Exit(1)
	}

	switch appMode {
	case mode.Server:
		fmt.Printf("Starting server...\n")
		startServer(appCtx)
	case mode.ServerConfGen:
		err := handlers.GenerateNewClientConf()
		if err != nil {
			log.Printf("failed to generate new client conf: %v", err)
		}
		return
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

func startClient(appCtx context.Context) {
	deps := client.NewDependencies(client_configuration.NewManager())
	depsErr := deps.Initialize()
	if depsErr != nil {
		log.Fatalf("init error: %s", depsErr)
	}

	routerFactory := client_factory.NewRouterFactory()

	runner := client.NewRunner(deps, routerFactory)
	runner.Run(appCtx)
}

func startServer(appCtx context.Context) {
	tunFactory := pal_factory.NewServerTunFactory()
	configurationManager := server_configuration.NewManager(server_configuration.NewServerResolver())
	conf, confErr := configurationManager.Configuration()
	if confErr != nil {
		log.Fatal(confErr)
	}

	deps := server.NewDependencies(tunFactory, *conf, server_configuration.NewEd25519KeyManager(conf, configurationManager))

	runner := server.NewRunner(deps)
	runner.Run(appCtx)
}

func printVersion(appCtx context.Context) {
	runner := version.NewRunner()
	runner.Run(appCtx)
}
