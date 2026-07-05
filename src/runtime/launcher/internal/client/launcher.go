package client

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	palClient "tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/logging"
	"tungo/infrastructure/tunnel/sessionplane/client_factory"
	appRuntime "tungo/runtime"
	runtimeClient "tungo/runtime/client"
)

func Run(ctx context.Context) error {
	runner, _, err := newRuntime()
	if err != nil {
		return err
	}
	return runner.Run(ctx, runtimeClient.RunOptions{})
}

func Start(ctx context.Context) (appRuntime.Session, error) {
	runner, conf, err := newRuntime()
	if err != nil {
		return nil, err
	}

	sessionCtx, cancel := context.WithCancel(ctx)
	readyCh := make(chan struct{})
	session := appRuntime.NewRunningSession(
		appRuntime.Info{
			Mode:      appRuntime.ModeClient,
			Endpoints: appRuntime.EndpointInfoFromClientConfiguration(conf),
			Protocol:  conf.Protocol,
		},
		readyCh,
		cancel,
	)

	go func() {
		session.Finish(runner.Run(sessionCtx, runtimeClient.RunOptions{ReadyCh: readyCh}))
	}()
	return session, nil
}

func newRuntime() (*runtimeClient.Runner, palClient.Configuration, error) {
	setupCrashLog()
	slog.Info("starting client")

	deps := runtimeClient.NewDependencies(palClient.NewManager())
	if err := deps.Initialize(); err != nil {
		return nil, palClient.Configuration{}, fmt.Errorf("init error: %w", err)
	}

	routerFactory := client_factory.NewRouterFactory()
	return runtimeClient.NewRunner(deps, routerFactory), deps.Configuration(), nil
}

func setupCrashLog() {
	configPath, err := palClient.NewDefaultResolver().Resolve()
	if err != nil {
		return
	}
	logging.SetCrashOutput(filepath.Join(filepath.Dir(configPath), "crash.log"))
}
