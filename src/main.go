package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"
	"tungo/application/commandline"
	"tungo/application/confgen"
	appConfiguration "tungo/application/configuration"
	"tungo/application/version"
	"tungo/domain/app"
	"tungo/infrastructure/PAL/signal"
	"tungo/infrastructure/logging"
	"tungo/infrastructure/telemetry/trafficstats"
	"tungo/presentation/elevation"
	"tungo/presentation/signals/shutdown"
	"tungo/presentation/ui/tui"
	"tungo/runtime"
)

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
		signal.NewDefaultProvider(),
		shutdown.NewNotifier(),
	)
	shutdownSignalHandler.Handle()
	var err error
	switch uiMode := app.CurrentUIMode(); uiMode {
	case app.CLI:
		err = runCLI(ctx)
	case app.TUI:
		err = runTUI(ctx)
	default:
		err = fmt.Errorf("unknown UI mode: %v", uiMode)
	}
	if err != nil {
		exitCode = showFatal(err)
	}
}

func runCLI(ctx context.Context) error {
	command, err := commandline.ParseCommand(os.Args[1:])
	if err != nil {
		fmt.Print(commandline.CommandUsage(app.Name))
		return fmt.Errorf("configuration error: %w", err)
	}
	if command.RequiresElevation {
		if err := requireElevation(); err != nil {
			return err
		}
	}
	switch command.Kind {
	case commandline.CommandVersion:
		version.Run()
		return nil
	case commandline.CommandServerConfigGenerate:
		return confgen.Run()
	case commandline.CommandRuntime:
		session, err := runtime.Start(ctx, command.RuntimeMode)
		if err != nil {
			return err
		}
		return runtimeErrOrNil(ctx, session.Wait())
	default:
		return fmt.Errorf("unhandled command kind: %v", command.Kind)
	}
}

func runtimeErrOrNil(ctx context.Context, err error) error {
	if err != nil && ctx.Err() == nil &&
		!errors.Is(err, context.Canceled) &&
		!errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return nil
}

func runTUI(ctx context.Context) error {
	if err := requireElevation(); err != nil {
		return err
	}
	configurationControls, err := appConfiguration.NewDefaultControls()
	if err != nil {
		return err
	}
	tuiUI, err := tui.New(configurationControls)
	if err != nil {
		return err
	}
	trafficCollector := trafficstats.NewCollector(time.Second, 0.35)
	trafficstats.SetGlobal(trafficCollector)
	go trafficCollector.Start(ctx)

	defer func() {
		tuiUI.Close()
		trafficstats.SetGlobal(nil)
	}()

	return tuiUI.Run(ctx)
}

func requireElevation() error {
	if elevation.IsElevated() {
		return nil
	}
	return fmt.Errorf(
		"%s must be run with admin privileges.\n%s",
		app.Name, elevation.Hint(),
	)
}

// showFatal displays a fatal error and returns the exit code.
// In TUI mode it shows a themed, dismissable screen; in CLI mode it logs.
func showFatal(err error) int {
	if app.CurrentUIMode() == app.TUI {
		tui.ShowFatalError(err.Error())
	} else {
		slog.Error("fatal error", "err", err)
	}
	return 1
}
