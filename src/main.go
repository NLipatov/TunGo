package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"
	"tungo/application/confgen"
	"tungo/domain/app"
	"tungo/domain/mode"
	"tungo/infrastructure/PAL/configuration/client"
	serverConf "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/PAL/signal"
	"tungo/infrastructure/PAL/stat"
	"tungo/infrastructure/PAL/platform"
	"tungo/infrastructure/PAL/tunnel/tun_server"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/tunnel/sessionplane/client_factory"
	"tungo/presentation/configuring"
	"tungo/presentation/elevation"
	clientConf "tungo/presentation/runners/client"
	runnersCommon "tungo/presentation/runners/common"
	"tungo/presentation/runners/server"
	"tungo/presentation/runners/version"
	"tungo/presentation/signals/shutdown"
	"tungo/presentation/ui/tui"
)

func main() {
	exitCode := 0
	defer func() { os.Exit(exitCode) }()

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
		log.Printf("Error: %s", err.Error())
	}
	return 1
}

// --- mode runners ---

func runServer(ctx context.Context, uiMode app.UIMode, resolver client.Resolver, manager serverConf.ConfigurationManager) error {
	setupCrashLog(resolver)
	if err := prepareServerKeys(manager); err != nil {
		return fmt.Errorf("key preparation failed: %w", err)
	}
	configPath, _ := resolver.Resolve()
	log.Printf("Starting server...")

	tunFactory := tun_server.NewServerTunFactory()

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

	workerFactory, err := tun_server.NewServerWorkerFactory(manager)
	if err != nil {
		return fmt.Errorf("failed to create worker factory: %w", err)
	}

	configWatcher := serverConf.NewConfigWatcher(
		manager,
		workerFactory.SessionRevoker(),
		workerFactory.AllowedPeersUpdater(),
		configPath,
		serverConf.DefaultWatchInterval,
		log.Default(),
	)
	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()
	go configWatcher.Watch(watchCtx)

	runner := server.NewRunner(
		uiMode,
		deps,
		workerFactory,
		tun_server.NewServerTrafficRouterFactory(),
	)
	return runner.Run(ctx)
}

func runServerConfGen(manager serverConf.ConfigurationManager) error {
	if err := prepareServerKeys(manager); err != nil {
		return fmt.Errorf("key preparation failed: %w", err)
	}
	gen := confgen.NewGenerator(manager, &primitives.DefaultKeyDeriver{})
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
	log.Printf("Starting client...")

	deps := clientConf.NewDependencies(client.NewManager())
	if err := deps.Initialize(); err != nil {
		return fmt.Errorf("init error: %w", err)
	}

	routerFactory := client_factory.NewRouterFactory()
	runner := clientConf.NewRunner(uiMode, deps, routerFactory)
	return runner.Run(ctx)
}

// --- helpers ---

func prepareServerKeys(manager serverConf.ConfigurationManager) error {
	keyManager := serverConf.NewX25519KeyManager(manager)
	if err := keyManager.PrepareKeys(); err != nil {
		return fmt.Errorf("could not prepare keys: %w", err)
	}
	return nil
}

func setupCrashLog(resolver client.Resolver) {
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
