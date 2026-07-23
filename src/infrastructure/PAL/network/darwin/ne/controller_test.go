//go:build darwin || ios

package ne

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

type observedNotReadyRuntime struct {
	readyObserved chan struct{}
	once          sync.Once
}

func (*observedNotReadyRuntime) Run(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (r *observedNotReadyRuntime) Ready() bool {
	r.once.Do(func() { close(r.readyObserved) })
	return false
}

func TestControllerStartRejectsInvalidDescriptor(t *testing.T) {
	controller := NewController()
	if err := controller.Start(-1); err == nil {
		t.Fatal("expected invalid descriptor error")
	}
}

func TestControllerWaitReadyRequiresStart(t *testing.T) {
	controller := NewController()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := controller.WaitReady(ctx); err == nil {
		t.Fatal("expected not-started error")
	}
}

func TestControllerStopIsIdempotent(t *testing.T) {
	controller := NewController()
	if err := controller.Stop(); err != nil {
		t.Fatalf("first Stop() error = %v", err)
	}
	if err := controller.Stop(); err != nil {
		t.Fatalf("second Stop() error = %v", err)
	}
}

func TestControllerStopInterruptsWaitReady(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	runtime := &observedNotReadyRuntime{readyObserved: make(chan struct{})}
	s := &session{
		runtime: runtime,
		cancel:  cancel,
		done:    done,
		state:   stateStarting,
		release: func() {},
	}
	go func() {
		<-ctx.Done()
		s.mu.Lock()
		s.state = stateStopped
		s.mu.Unlock()
		close(done)
	}()

	controller := NewController()
	controller.session = s

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- controller.WaitReady(context.Background())
	}()
	select {
	case <-runtime.readyObserved:
	case <-time.After(time.Second):
		t.Fatal("WaitReady() did not inspect runtime readiness")
	}

	if err := controller.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	select {
	case err := <-waitDone:
		if err == nil || !strings.Contains(err.Error(), "stopped before becoming ready") {
			t.Fatalf("WaitReady() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("WaitReady() did not observe the stop")
	}
}
