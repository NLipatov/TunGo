package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"tungo/presentation"
	"tungo/presentation/elevation"
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

	var mode string
	if len(os.Args) < 2 {
		mode = strings.
			ToLower(strings.
				TrimSpace(promptForMode()))
	} else {
		mode = os.Args[1]
	}

	switch mode {
	case ServerMode:
		fmt.Printf("%s Starting server...\n", ServerIcon)
		presentation.StartServer()
	case ClientMode:
		fmt.Printf("%s️ Starting client...\n", ClientIcon)
		presentation.StartClient(appCtx)
	default:
		fmt.Printf("❌ Unknown mode: %s\n", mode)
		printUsage()
		os.Exit(1)
	}
}

func promptForMode() string {
	fmt.Printf("✨ Welcome to %s!", PackageName)
	fmt.Println("Please select mode:")
	fmt.Printf("\t %s - Server %s\n", ServerMode, ServerIcon)
	fmt.Printf("\t %s - Client %s\n", ClientMode, ClientIcon)
	fmt.Print("👉 Your choice: ")

	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text())
	}

	return ""
}

func printUsage() {
	fmt.Printf(`Usage: %s <mode>
Modes:
  %s  - Server %s
  %s  - Client %s
`, PackageName, ServerMode, ServerIcon, ClientMode, ClientIcon)
}
