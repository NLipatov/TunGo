package common

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestWaitForRuntimeSessionEnd_WorkerFinishesFirst_LogsUnexpectedUIError(t *testing.T) {
	uiCh := make(chan RuntimeUIResult, 1)
	workerCh := make(chan error)
	logged := false
	canceled := make(chan struct{})
	var cancelOnce sync.Once

	done := make(chan error, 1)
	go func() {
		done <- WaitForRuntimeSessionEnd(
			func() { cancelOnce.Do(func() { close(canceled) }) },
			uiCh,
			workerCh,
			func(error) bool { return false },
			func(error) { logged = true },
		)
	}()
	go func() {
		workerCh <- errors.New("worker failed")
	}()
	<-canceled
	uiCh <- RuntimeUIResult{Err: errors.New("ui failed")}

	err := <-done
	if err == nil || !strings.Contains(err.Error(), "worker failed") {
		t.Fatalf("expected worker error, got %v", err)
	}
	if !logged {
		t.Fatal("expected runtime UI error callback to be called")
	}
}

func TestWaitForRuntimeSessionEnd_UserExit_ReturnsContextCanceled(t *testing.T) {
	uiCh := make(chan RuntimeUIResult, 1)
	workerCh := make(chan error, 1)

	uiCh <- RuntimeUIResult{Err: errors.New("user exit")}
	workerCh <- context.Canceled

	err := WaitForRuntimeSessionEnd(
		func() {},
		uiCh,
		workerCh,
		func(err error) bool { return err != nil && err.Error() == "user exit" },
		nil,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestWaitForRuntimeSessionEnd_ReconfigureRequested(t *testing.T) {
	uiCh := make(chan RuntimeUIResult, 1)
	workerCh := make(chan error)
	canceled := make(chan struct{})

	done := make(chan error, 1)
	go func() {
		done <- WaitForRuntimeSessionEnd(
			func() { close(canceled) },
			uiCh,
			workerCh,
			func(error) bool { return false },
			nil,
		)
	}()
	uiCh <- RuntimeUIResult{UserQuit: true}
	go func() {
		<-canceled
		workerCh <- nil
	}()

	err := <-done
	if !errors.Is(err, ErrReconfigureRequested) {
		t.Fatalf("expected ErrReconfigureRequested, got %v", err)
	}
}
