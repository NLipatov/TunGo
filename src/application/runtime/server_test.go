package runtime

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"

	"tungo/application/configuration"
	"tungo/application/network/routing"
	"tungo/infrastructure/settings"
)

type serverTestControl struct {
	watchStarted int32
}

func (*serverTestControl) ServerRuntimeConfiguration() (configuration.ServerRuntimeConfiguration, error) {
	return configuration.ServerRuntimeConfiguration{}, nil
}

func (c *serverTestControl) WatchServerRuntimeConfiguration(
	ctx context.Context,
	_ configuration.ServerSessionRevoker,
	_ configuration.ServerAllowedPeersUpdater,
) {
	atomic.AddInt32(&c.watchStarted, 1)
	<-ctx.Done()
}

func TestServerRun_StartsWatcher(t *testing.T) {
	control := &serverTestControl{}
	runtime := newServerTestRuntime(
		&serverTestTunManager{
			createErrByProto: map[settings.Protocol]error{settings.TCP: errServerTest},
		},
		configuration.ServerRuntimeConfiguration{
			EnableTCP:   true,
			TCPSettings: settings.Settings{Protocol: settings.TCP},
		},
	)
	runtime.control = control

	err := runtime.Run(context.Background())
	if !errors.Is(err, errServerTest) {
		t.Fatalf("Run() error = %v, want %v", err, errServerTest)
	}
	if atomic.LoadInt32(&control.watchStarted) == 0 {
		t.Fatal("configuration watcher was not started")
	}
}

func TestServerRun_NormalizesCancellation(t *testing.T) {
	runtime := newServerTestRuntime(
		&serverTestTunManager{},
		configuration.ServerRuntimeConfiguration{
			EnableTCP:   true,
			TCPSettings: settings.Settings{Protocol: settings.TCP},
		},
	)
	runtime.workerFactory = serverTestWorkerFactory{
		create: func(context.Context, io.ReadWriteCloser, settings.Settings) (routing.Worker, error) {
			return serverTestWorker{}, nil
		},
	}
	runtime.routerFactory = serverTestRouterFactory{
		make: func(routing.Worker) routing.Router {
			return serverTestRouter{route: func(context.Context) error { return context.Canceled }}
		},
	}

	if err := runtime.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v, want clean stop", err)
	}
}

func TestServerRun_CanceledContextWinsOverRuntimeError(t *testing.T) {
	runtime := newServerTestRuntime(
		&serverTestTunManager{
			createErrByProto: map[settings.Protocol]error{settings.TCP: errServerTest},
		},
		configuration.ServerRuntimeConfiguration{
			EnableTCP:   true,
			TCPSettings: settings.Settings{Protocol: settings.TCP},
		},
	)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := runtime.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v, want clean stop", err)
	}
}
