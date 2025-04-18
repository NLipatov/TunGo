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
	"tungo/infrastructure/routing/client_routing/factory"
	"tungo/presentation/configuring"
	"tungo/presentation/elevation"
	"tungo/presentation/runners/client"
	"tungo/presentation/runners/server"
	"tungo/settings/client_configuration"
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
		server.StartServer(appCtx)
	case mode.Client:
		fmt.Printf("Starting client...\n")
		startClient(appCtx)
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

	routerFactory := factory.NewRouterFactory()

	runner := client.NewRunner(deps, routerFactory)
	runner.Run(appCtx)
}
