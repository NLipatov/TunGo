package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"tungo/internal/client"
	"tungo/internal/commandline"
	clientconfig "tungo/internal/config/client"
	serverconfig "tungo/internal/config/server"
	"tungo/internal/daemon/systemd"
	"tungo/internal/elevation"
	"tungo/internal/logging"
	"tungo/internal/mode"
	"tungo/internal/platform"
	"tungo/internal/platform/command"
	"tungo/internal/product"
	"tungo/internal/server"
	"tungo/internal/shutdown"
	"tungo/internal/trafficstats"
	"tungo/internal/ui/tui"
)

func main() {
	exitCode := 0
	defer func() { os.Exit(exitCode) }()

	logger := logging.NewLogger(slog.LevelInfo)
	slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	shutdown.Watch(ctx, cancel)

	if isTUI() {
		if err := runTUI(ctx); err != nil {
			tui.ShowFatalError(err.Error())
			exitCode = 1
		}
	} else {
		if err := runCLI(ctx); err != nil {
			slog.Error("fatal error", "err", err)
			exitCode = 1
		}
	}
}

func runCLI(ctx context.Context) error {
	command, err := commandline.ParseCommand(os.Args[1:])
	if err != nil {
		fmt.Print(commandline.CommandUsage(product.Name))
		return fmt.Errorf("configuration error: %w", err)
	}
	if command.RequiresElevation {
		if err := requireElevation(); err != nil {
			return err
		}
	}
	switch command.Kind {
	case commandline.CommandVersion:
		fmt.Printf("%s %s\n", product.Name, product.Version)
		return nil
	case commandline.CommandServerConfigGenerate:
		if !platform.ServerModeSupported() {
			return fmt.Errorf("server configuration is not supported")
		}
		generated, err := serverconfig.DefaultFile().GenerateClient()
		if err != nil {
			return fmt.Errorf("configuration generation failed: %w", err)
		}
		fmt.Println(generated.JSON)
		return nil
	case commandline.CommandRuntime:
		switch command.RuntimeMode {
		case mode.Client:
			configuration, err := clientconfig.Files().Active()
			if err != nil {
				return fmt.Errorf("failed to load client configuration: %w", err)
			}
			runningClient, err := client.New(configuration)
			if err != nil {
				return err
			}
			return runningClient.Run(ctx)
		case mode.Server:
			runningServer, err := server.New(serverconfig.DefaultFile())
			if err != nil {
				return err
			}
			return runningServer.Run(ctx)
		default:
			return fmt.Errorf("invalid runtime mode: %v", command.RuntimeMode)
		}
	default:
		return fmt.Errorf("unhandled command kind: %v", command.Kind)
	}
}

func runTUI(ctx context.Context) error {
	if err := requireElevation(); err != nil {
		return err
	}
	clientConfigurations := clientconfig.Files()
	var serverFile *serverconfig.File
	if platform.ServerModeSupported() {
		serverFile = serverconfig.DefaultFile()
	}
	var daemonControl systemd.Control
	systemdControl := systemd.NewUnitInstaller(command.New())
	if systemdControl.Available() {
		daemonControl = systemdControl
	}
	tuiUI, err := tui.New(clientConfigurations, serverFile, daemonControl)
	if err != nil {
		return err
	}
	trafficCollector := trafficstats.NewCollector(time.Second, 0.35)
	trafficstats.SetGlobal(trafficCollector)
	go trafficCollector.Start(ctx)

	defer trafficstats.SetGlobal(nil)

	return tuiUI.Run(ctx)
}

func requireElevation() error {
	if elevation.IsElevated() {
		return nil
	}
	return fmt.Errorf(
		"%s must be run with admin privileges.\n%s",
		product.Name, elevation.Hint(),
	)
}

func isTUI() bool {
	return len(os.Args) < 2
}
