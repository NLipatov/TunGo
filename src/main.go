package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"tungo/domain/mode"
	"tungo/presentation"
	"tungo/presentation/elevation"
	"tungo/presentation/mode_selection"
)

const (
	PackageName = "tungo"
	ServerMode  = "s"
	ClientMode  = "c"
	ServerIcon  = "🌐"
	ClientIcon  = "🖥️"
)

func main() {
	processElevation := elevation.NewProcessElevation()
	if !processElevation.IsElevated() {
		fmt.Printf("⚠️ Warning: %s must be run with admin privileges", PackageName)
		return
	}

	appCtx, appCtxCancel := context.WithCancel(context.Background())
	defer appCtxCancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-sigChan
		fmt.Println("\n⏹️  Interrupt received. Shutting down...")
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
		fmt.Printf("%s Starting server...\n", ServerIcon)
		presentation.StartServer(appCtx)
	case mode.Client:
		fmt.Printf("%s️ Starting client...\n", ClientIcon)
		presentation.StartClient(appCtx)
	default:
		fmt.Printf("❌ Unknown mode: %s\n", selectedMode)
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
