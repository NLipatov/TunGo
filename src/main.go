package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"tungo/application"
	"tungo/application/commandline"
	"tungo/application/configuration"
	"tungo/application/version"
	"tungo/infrastructure/PAL/exec_commander"
	"tungo/infrastructure/PAL/service_management/linux/systemd"
	"tungo/infrastructure/logging"
	"tungo/infrastructure/telemetry/trafficstats"
	tunnelClient "tungo/infrastructure/tunnel/client"
	tunnelServer "tungo/infrastructure/tunnel/server"
	"tungo/presentation/elevation"
	"tungo/presentation/signals/shutdown"
	"tungo/presentation/ui/tui"
)

const appName = "tungo"

func main() {
	exitCode := 0
	defer func() { os.Exit(exitCode) }()

	logger := logging.NewLogger(slog.LevelInfo)
	slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	shutdownSignalHandler := shutdown.NewHandler(
		ctx,
		cancel,
		shutdown.NewNotifier(),
	)
	shutdownSignalHandler.Handle()
	var err error
	if isTUI() {
		err = runTUI(ctx)
	} else {
		err = runCLI(ctx)
	}
	if err != nil {
		exitCode = showFatal(err)
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
		serverControl := configuration.NewServerControl()
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
		case application.ModeClient:
			client, err := tunnelClient.New()
			if err != nil {
				return err
			}
			return client.Run(ctx)
		case application.ModeServer:
			server, err := tunnelServer.New()
			if err != nil {
				return err
			}
			return server.Run(ctx)
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
	configurationControls := configuration.NewControls()
	var daemonControl systemd.Control
	systemdControl := systemd.NewUnitInstaller(exec_commander.NewExecCommander())
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

// showFatal displays a fatal error and returns the exit code.
// In TUI mode it shows a themed, dismissable screen; in CLI mode it logs.
func showFatal(err error) int {
	if isTUI() {
		tui.ShowFatalError(err.Error())
	} else {
		slog.Error("fatal error", "err", err)
	}
	return 1
}

func isTUI() bool {
	return len(os.Args) < 2
}
