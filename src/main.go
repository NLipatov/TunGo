package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"tungo/domain/mode"
	"tungo/presentation"
	"tungo/presentation/configuration_selection"
	"tungo/presentation/elevation"
	"tungo/presentation/mode_selection"
	"tungo/settings/client_configuration"
)

const (
	PackageName = "tungo"
	ServerMode  = "s"
	ClientMode  = "c"
)

func main() {
	processElevation := elevation.NewProcessElevation()
	if !processElevation.IsElevated() {
		fmt.Printf("Warning: %s must be run with admin privileges", PackageName)
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

	am := mode_selection.NewPromptAppMode(os.Args)
	selectedMode, selectedModeErr := am.Mode()
	if selectedModeErr != nil {
		fmt.Print(selectedModeErr)
		os.Exit(1)
	}

	switch selectedMode {
	case mode.Server:
		fmt.Printf("Starting server...\n")
		presentation.StartServer(appCtx)
	case mode.Client:
		confResolver := client_configuration.NewDefaultResolver()
		confSelector := configuration_selection.
			NewSelectableConfiguration(client_configuration.NewDefaultObserver(confResolver),
				client_configuration.NewDefaultSelector(confResolver))
		selectConfigurationErr := confSelector.SelectConfiguration()
		if selectConfigurationErr != nil {
			log.Fatal(selectConfigurationErr)
		}
		fmt.Printf("Starting client...\n")
		presentation.StartClient(appCtx)
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf(`Usage: %s <mode>
Modes:
  %s  - Server
  %s  - Client
`, PackageName, ServerMode, ClientMode)
}
