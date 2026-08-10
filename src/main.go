package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"tungo/internal/client"
	"tungo/internal/commandline"
	"tungo/internal/config"
	"tungo/internal/daemon/systemd"
	"tungo/internal/elevation"
	"tungo/internal/logging"
	"tungo/internal/platform/command"
	"tungo/internal/server"
	"tungo/internal/shutdown"
	"tungo/internal/trafficstats"
	"tungo/internal/ui/tui"
	"tungo/internal/version"
)

const appName = "tungo"

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
		fmt.Print(commandline.CommandUsage(appName))
		return fmt.Errorf("configuration error: %w", err)
	}
	if command.RequiresElevation {
		if err := requireElevation(); err != nil {
			return err
		}
	}
	switch command.Kind {
	case commandline.CommandVersion:
		fmt.Printf("%s %s\n", appName, version.Current())
		return nil
	case commandline.CommandServerConfigGenerate:
		serverControl := config.NewServerControl()
		if serverControl == nil {
			return fmt.Errorf("server configuration is not supported")
		}
		generated, err := serverControl.GenerateClientConfiguration()
		if err != nil {
			return fmt.Errorf("configuration generation failed: %w", err)
		}
		fmt.Println(generated.JSON)
		return nil
	case commandline.CommandRuntime:
		switch command.RuntimeMode {
		case config.ModeClient:
			runningClient, err := client.New()
			if err != nil {
				return err
			}
			return runningClient.Run(ctx)
		case config.ModeServer:
			runningServer, err := server.New()
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
	configurationControls := config.NewControls()
	var daemonControl systemd.Control
	systemdControl := systemd.NewUnitInstaller(command.New())
	if systemdControl.Available() {
		daemonControl = systemdControl
	}
	tuiUI, err := tui.New(configurationControls, daemonControl)
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
		appName, elevation.Hint(),
	)
}

func isTUI() bool {
	return len(os.Args) < 2
}
