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
	"tungo/infrastructure/PAL/tun_server"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/telemetry/trafficstats"
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

const (
	// configWatchInterval is how often the server checks for AllowedPeers changes.
	// Sessions for removed/disabled peers are revoked on detection.
	configWatchInterval = 30 * time.Second
)

func main() {
	exitCode := 0
	defer func() { os.Exit(exitCode) }()

	appCtx, appCtxCancel := context.WithCancel(context.Background())
	defer appCtxCancel()

	if len(os.Args) < 2 {
		trafficCollector := trafficstats.NewCollector(time.Second, 0.35)
		trafficstats.SetGlobal(trafficCollector)
		defer trafficstats.SetGlobal(nil)
		go trafficCollector.Start(appCtx)

		tui.EnableRuntimeLogCapture(1200)
		defer tui.DisableRuntimeLogCapture()
	}

	shutdownSignalHandler := shutdown.NewHandler(
		appCtx,
		appCtxCancel,
		signal.NewDefaultProvider(),
		shutdown.NewNotifier(),
	)
	shutdownSignalHandler.Handle()

	if err := run(appCtx); err != nil {
		exitCode = showFatal(err)
	}
}

func run(ctx context.Context) error {
	processElevation := elevation.NewProcessElevation()
	if !processElevation.IsElevated() {
		return fatal(
			"Insufficient privileges",
			fmt.Sprintf(
				"%s must be run with admin privileges.\n"+
					"Please restart with elevated permissions (e.g. 'Run as Administrator' on Windows, or 'sudo' on Linux/macOS).",
				app.Name,
			),
		)
	}

	serverResolver := serverConf.NewServerResolver()
	configurationManager, err := serverConf.NewManager(serverResolver, stat.NewDefaultStat())
	if err != nil {
		return fatal("Configuration error", err.Error())
	}

	configuratorFactory := configuring.NewConfigurationFactory(configurationManager)
	configurator := configuratorFactory.Configurator()

	for ctx.Err() == nil {
		appMode, err := configurator.Configure(ctx)
		if err != nil {
			if errors.Is(err, configuring.ErrUserExit) {
				return nil
			}
			return fatal("Configuration error", err.Error())
		}

		switch appMode {
		case mode.Server:
			setupCrashLog(serverResolver)
			if err := prepareServerKeys(configurationManager); err != nil {
				return fatal("Key preparation failed", err.Error())
			}
			serverConfigPath, _ := serverResolver.Resolve()
			log.Printf("Starting server...")
			err := startServer(ctx, configurationManager, serverConfigPath)
			if errors.Is(err, runnersCommon.ErrReconfigureRequested) {
				continue
			}
			if err != nil && ctx.Err() == nil {
				return fatalWithCode("Server error", err.Error(), 2)
			}
			return nil

		case mode.ServerConfGen:
			if err := prepareServerKeys(configurationManager); err != nil {
				return fatal("Key preparation failed", err.Error())
			}
			gen := confgen.NewGenerator(configurationManager, &primitives.DefaultKeyDeriver{})
			conf, err := gen.Generate()
			if err != nil {
				return fatal("Configuration generation failed", err.Error())
			}
			data, err := json.MarshalIndent(conf, "", "  ")
			if err != nil {
				return fatal("Configuration generation failed", err.Error())
			}
			fmt.Println(string(data))
			return nil

		case mode.Client:
			setupCrashLog(client.NewDefaultResolver())
			log.Printf("Starting client...")
			err := startClient(ctx)
			if errors.Is(err, runnersCommon.ErrReconfigureRequested) {
				continue
			}
			if err != nil && ctx.Err() == nil {
				return fatalWithCode("Client error", err.Error(), 2)
			}
			return nil

		case mode.Version:
			printVersion(ctx)
			return nil

		default:
			return fatal("Internal error", fmt.Sprintf("invalid app mode: %v", appMode))
		}
	}
	return nil
}

// --- fatal error type ---

type fatalError struct {
	title   string
	message string
	code    int
}

func (e fatalError) Error() string {
	return e.title + ": " + e.message
}

func fatal(title, message string) fatalError {
	return fatalError{title: title, message: message, code: 1}
}

func fatalWithCode(title, message string, code int) fatalError {
	return fatalError{title: title, message: message, code: code}
}

// showFatal displays a fatal error and returns the exit code.
// In TUI mode it shows a themed, dismissable screen; in CLI mode it logs.
func showFatal(err error) int {
	var fe fatalError
	if !errors.As(err, &fe) {
		fe = fatalError{title: "Error", message: err.Error(), code: 1}
	}
	if len(os.Args) < 2 {
		tui.ShowFatalError(fe.title, fe.message)
	} else {
		log.Printf("%s: %s", fe.title, fe.message)
	}
	return fe.code
}

// --- helpers ---

func prepareServerKeys(configurationManager serverConf.ConfigurationManager) error {
	keyManager := serverConf.NewX25519KeyManager(configurationManager)
	if err := keyManager.PrepareKeys(); err != nil {
		return fmt.Errorf("could not prepare keys: %w", err)
	}
	return nil
}

func startClient(appCtx context.Context) error {
	deps := clientConf.NewDependencies(client.NewManager())
	depsErr := deps.Initialize()
	if depsErr != nil {
		return fmt.Errorf("init error: %w", depsErr)
	}

	routerFactory := client_factory.NewRouterFactory()

	runner := clientConf.NewRunner(deps, routerFactory)
	return runner.Run(appCtx)
}

func startServer(
	ctx context.Context,
	configurationManager serverConf.ConfigurationManager,
	configPath string,
) error {
	tunFactory := tun_server.NewServerTunFactory()

	conf, confErr := configurationManager.Configuration()
	if confErr != nil {
		return fmt.Errorf("failed to load server configuration: %w", confErr)
	}

	deps := server.NewDependencies(
		tunFactory,
		*conf,
		serverConf.NewX25519KeyManager(configurationManager),
		configurationManager,
	)

	workerFactory, err := tun_server.NewServerWorkerFactory(configurationManager)
	if err != nil {
		return fmt.Errorf("failed to create worker factory: %w", err)
	}

	// Start ConfigWatcher to revoke sessions and update AllowedPeers at runtime
	configWatcher := serverConf.NewConfigWatcher(
		configurationManager,
		workerFactory.SessionRevoker(),
		workerFactory.AllowedPeersUpdater(),
		configPath,
		configWatchInterval,
		log.Default(),
	)
	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()
	go configWatcher.Watch(watchCtx)

	runner := server.NewRunner(
		deps,
		workerFactory,
		tun_server.NewServerTrafficRouterFactory(),
	)
	return runner.Run(ctx)
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
