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
	"tungo/presentation"
	"tungo/presentation/configuration_provider"
	"tungo/presentation/elevation"
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

	configuratorFactory := configuration_provider.NewConfigurationFactory()
	configurator := configuratorFactory.Configurator()
	appMode, appModeErr := configurator.Configure()
	if appModeErr != nil {
		log.Printf("%v", appModeErr)
		os.Exit(1)
	}

	switch appMode {
	case mode.Server:
		fmt.Printf("Starting server...\n")
		presentation.StartServer(appCtx)
	case mode.Client:
		fmt.Printf("Starting client...\n")
		presentation.StartClient(appCtx)
	default:
		log.Printf("invalid app mode: %v", appMode)
		os.Exit(1)
	}
}
