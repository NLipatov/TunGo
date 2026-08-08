package runtime

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"tungo/application/configuration"
)

var errServerTest = errors.New("boom")

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

func TestServerRunSignalsReadyAndStartsWatcher(t *testing.T) {
	control := &serverTestControl{}
	runtime := &serverRuntime{
		control: control,
		runServer: func(_ context.Context, ready func()) error {
			ready()
			return errServerTest
		},
	}

	if err := runtime.Run(context.Background()); !errors.Is(err, errServerTest) {
		t.Fatalf("Run() error = %v, want %v", err, errServerTest)
	}
	if !runtime.Ready() {
		t.Fatal("runtime did not become ready")
	}
	if atomic.LoadInt32(&control.watchStarted) == 0 {
		t.Fatal("configuration watcher was not started")
	}
}

func TestServerRunFailureBeforeReady(t *testing.T) {
	runtime := &serverRuntime{
		control:   &serverTestControl{},
		runServer: func(context.Context, func()) error { return errServerTest },
	}

	if err := runtime.Run(context.Background()); !errors.Is(err, errServerTest) {
		t.Fatalf("Run() error = %v, want %v", err, errServerTest)
	}
	if runtime.Ready() {
		t.Fatal("runtime became ready after startup failure")
	}
}

func TestServerRunNormalizesCancellation(t *testing.T) {
	runtime := &serverRuntime{
		control:   &serverTestControl{},
		runServer: func(context.Context, func()) error { return context.Canceled },
	}

	if err := runtime.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v, want clean stop", err)
	}
}

func TestServerCanceledContextWinsOverRuntimeError(t *testing.T) {
	runtime := &serverRuntime{
		control:   &serverTestControl{},
		runServer: func(context.Context, func()) error { return errServerTest },
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := runtime.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v, want clean stop", err)
	}
}
