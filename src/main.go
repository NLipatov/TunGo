package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"
	"tungo/application/confgen"
	"tungo/domain/app"
	"tungo/domain/mode"
	"tungo/infrastructure/PAL/configuration"
	"tungo/infrastructure/PAL/configuration/client"
	serverConf "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/PAL/platform"
	"tungo/infrastructure/PAL/signal"
	"tungo/infrastructure/PAL/stat"
	tunnelServer "tungo/infrastructure/PAL/tunnel/server"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/logging"
	"tungo/infrastructure/network/host_resolver"
	"tungo/infrastructure/tunnel/sessionplane/client_factory"
	"tungo/presentation/configuring"
	"tungo/presentation/elevation"
	"tungo/presentation/signals/shutdown"
	"tungo/presentation/ui/tui"
	runnersCommon "tungo/runtime"
	clientConf "tungo/runtime/client"
	"tungo/runtime/server"
	"tungo/runtime/version"
)

func main() {
	exitCode := 0
	defer func() { os.Exit(exitCode) }()

	setupSlog()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	shutdownSignalHandler := shutdown.NewHandler(
		ctx,
		cancel,
		signal.NewDefaultProvider(),
		shutdown.NewNotifier(),
	)
	shutdownSignalHandler.Handle()

	if err := run(ctx); err != nil {
		exitCode = showFatal(err)
	}
}

func setupSlog() {
	logger := logging.NewLogger(slog.LevelInfo)
	slog.SetDefault(logger)
}

func run(ctx context.Context) error {
	if !elevation.IsElevated() {
		return fmt.Errorf(
			"%s must be run with admin privileges.\n%s",
			app.Name, elevation.Hint(),
		)
	}

	serverResolver := serverConf.NewServerResolver()
	configurationManager, err := serverConf.NewManager(serverResolver, stat.NewDefaultStat())
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	uiMode := app.CurrentUIMode()
	configuratorFactory := configuring.NewConfigurationFactory(uiMode, configurationManager, platform.Capabilities().ServerModeSupported())
	configurator, cleanup := configuratorFactory.Configurator(ctx)
	defer cleanup()

	for ctx.Err() == nil {
		appMode, err := configurator.Configure(ctx)
		if err != nil {
			if errors.Is(err, configuring.ErrUserExit) || ctx.Err() != nil {
				return nil
			}
			if errors.Is(err, configuring.ErrSessionClosed) {
				return fmt.Errorf("ui session ended during shutdown: %w", err)
			}
			return fmt.Errorf("configuration error: %w", err)
		}

		var runErr error
		switch appMode {
		case mode.Server:
			runErr = runServer(ctx, uiMode, serverResolver, configurationManager)
		case mode.ServerConfGen:
			return runServerConfGen(configurationManager)
		case mode.Client:
			runErr = runClient(ctx, uiMode)
		case mode.Version:
			printVersion(ctx)
			return nil
		default:
			return fmt.Errorf("invalid app mode: %v", appMode)
		}
		if errors.Is(runErr, runnersCommon.ErrReconfigureRequested) {
			continue
		}
		if runErr != nil && ctx.Err() == nil &&
			!errors.Is(runErr, context.Canceled) &&
			!errors.Is(runErr, context.DeadlineExceeded) {
			return runErr
		}
		return nil
	}
	return nil
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

// --- mode runners ---

func runServer(ctx context.Context, uiMode app.UIMode, resolver configuration.Resolver, manager serverConf.ConfigurationManager) error {
	setupCrashLog(resolver)
	if err := prepareServerKeys(manager); err != nil {
		return fmt.Errorf("key preparation failed: %w", err)
	}
	configPath, _ := resolver.Resolve()
	slog.Info("starting server", "config_path", configPath)

	tunFactory := tunnelServer.NewTunFactory()

	conf, confErr := manager.Configuration()
	if confErr != nil {
		return fmt.Errorf("failed to load server configuration: %w", confErr)
	}

	deps := server.NewDependencies(
		tunFactory,
		*conf,
		serverConf.NewX25519KeyManager(manager),
		manager,
	)

	runtime, err := tunnelServer.NewRuntime(manager)
	if err != nil {
		return fmt.Errorf("failed to create server runtime: %w", err)
	}

	workerFactory, err := tunnelServer.NewWorkerFactory(runtime, manager)
	if err != nil {
		return fmt.Errorf("failed to create worker factory: %w", err)
	}

	configWatcher := serverConf.NewConfigWatcher(
		manager,
		runtime.SessionRevoker(),
		runtime.AllowedPeersUpdater(),
		configPath,
		serverConf.DefaultWatchInterval,
		logging.NewStdLogger(slog.LevelInfo),
	)
	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()
	go configWatcher.Watch(watchCtx)

	runner := server.NewRunner(
		deps,
		workerFactory,
		tunnelServer.NewTrafficRouterFactory(),
	)
	if uiMode == app.TUI {
		return runServerRuntimeDashboard(ctx, runner, *conf)
	}
	return runner.Run(ctx)
}

func runServerConfGen(manager serverConf.ConfigurationManager) error {
	if err := prepareServerKeys(manager); err != nil {
		return fmt.Errorf("key preparation failed: %w", err)
	}
	gen := confgen.NewGenerator(manager, &primitives.DefaultKeyDeriver{}, host_resolver.NewDialResolver())
	conf, err := gen.Generate()
	if err != nil {
		return fmt.Errorf("configuration generation failed: %w", err)
	}
	data, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		return fmt.Errorf("configuration generation failed: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func runClient(ctx context.Context, uiMode app.UIMode) error {
	setupCrashLog(client.NewDefaultResolver())
	slog.Info("starting client")

	deps := clientConf.NewDependencies(client.NewManager())
	if err := deps.Initialize(); err != nil {
		return fmt.Errorf("init error: %w", err)
	}

	routerFactory := client_factory.NewRouterFactory()
	runner := clientConf.NewRunner(deps, routerFactory)
	if uiMode == app.TUI {
		return runClientRuntimeDashboard(ctx, runner, deps.Configuration())
	}
	return runner.Run(ctx)
}

func runClientRuntimeDashboard(ctx context.Context, runner *clientConf.Runner, conf client.Configuration) error {
	for ctx.Err() == nil {
		err := runClientRuntimeDashboardSession(ctx, runner, conf)
		switch {
		case err == nil:
			return nil
		case errors.Is(err, context.Canceled):
			return context.Canceled
		case errors.Is(err, runnersCommon.ErrReconfigureRequested):
			return runnersCommon.ErrReconfigureRequested
		default:
			slog.Warn("session error, reconnecting", "err", err)
			timer := time.NewTimer(500 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return context.Canceled
			case <-timer.C:
			}
		}
	}
	return context.Canceled
}

func runClientRuntimeDashboardSession(ctx context.Context, runner *clientConf.Runner, conf client.Configuration) error {
	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	readyCh := make(chan struct{})
	workerErrCh := make(chan error, 1)
	go func() {
		workerErrCh <- runner.RunSession(sessionCtx, clientConf.SessionOptions{ReadyCh: readyCh})
	}()

	uiResultCh := make(chan tui.RuntimeUIResult, 1)
	go func() {
		userQuit, err := tui.RunRuntimeDashboard(sessionCtx, tui.RuntimeModeClient, tui.RuntimeUIOptions{
			ReadyCh:   readyCh,
			Endpoints: runnersCommon.EndpointInfoFromClientConfiguration(conf),
			Protocol:  conf.Protocol,
		})
		uiResultCh <- tui.RuntimeUIResult{UserQuit: userQuit, Err: err}
	}()

	return tui.WaitForRuntimeSessionEnd(
		cancel,
		uiResultCh,
		workerErrCh,
		func(err error) bool { return errors.Is(err, tui.ErrUserExit) },
		func(err error) { slog.Error("runtime UI error", "err", err) },
	)
}

func runServerRuntimeDashboard(ctx context.Context, runner *server.Runner, conf serverConf.Configuration) error {
	runtimeCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	workerErrCh := make(chan error, 1)
	go func() {
		workerErrCh <- runner.Run(runtimeCtx)
	}()

	uiResultCh := make(chan tui.RuntimeUIResult, 1)
	go func() {
		userQuit, err := tui.RunRuntimeDashboard(runtimeCtx, tui.RuntimeModeServer, tui.RuntimeUIOptions{
			ReadyCh:   closedReadyCh(),
			Endpoints: runnersCommon.EndpointInfoFromServerConfiguration(conf),
		})
		uiResultCh <- tui.RuntimeUIResult{UserQuit: userQuit, Err: err}
	}()

	return tui.WaitForRuntimeSessionEnd(
		cancel,
		uiResultCh,
		workerErrCh,
		func(err error) bool { return errors.Is(err, tui.ErrUserExit) },
		func(err error) { slog.Error("runtime UI error", "err", err) },
	)
}

func closedReadyCh() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

// --- helpers ---

func prepareServerKeys(manager serverConf.ConfigurationManager) error {
	keyManager := serverConf.NewX25519KeyManager(manager)
	if err := keyManager.PrepareKeys(); err != nil {
		return fmt.Errorf("could not prepare keys: %w", err)
	}
	return nil
}

func setupCrashLog(resolver configuration.Resolver) {
	configPath, err := resolver.Resolve()
	if err != nil {
		return
	}
	crashPath := filepath.Join(filepath.Dir(configPath), "crash.log")
	f, err := os.OpenFile(crashPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return
	}
	info, _ := f.Stat()
	if info != nil && info.Size() > 0 {
		_, _ = fmt.Fprintf(f, "\n--- crash at %s ---\n\n", time.Now().Format(time.RFC3339))
	}
	_ = debug.SetCrashOutput(f, debug.CrashOptions{})
}

func printVersion(appCtx context.Context) {
	runner := version.NewRunner()
	runner.Run(appCtx)
}
