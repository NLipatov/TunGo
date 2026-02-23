package shutdown

import (
	"context"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

//
// Mocks
//

// ShutdownHandlerMockNotifier mocks the Notifier interface.
type ShutdownHandlerMockNotifier struct {
	notifyCalled  int32
	stopCalled    int32
	notifyChan    chan<- os.Signal
	stopChan      chan<- os.Signal
	notifySignals []os.Signal
}

func (m *ShutdownHandlerMockNotifier) Notify(c chan<- os.Signal, sig ...os.Signal) {
	atomic.AddInt32(&m.notifyCalled, 1)
	m.notifyChan = c
	m.notifySignals = sig
}

func (m *ShutdownHandlerMockNotifier) Stop(c chan<- os.Signal) {
	atomic.AddInt32(&m.stopCalled, 1)
	m.stopChan = c
}

// ShutdownHandlerMockProvider mocks palSignal.Provider.
type ShutdownHandlerMockProvider struct {
	signals []os.Signal
}

func (p *ShutdownHandlerMockProvider) ShutdownSignals() []os.Signal {
	return p.signals
}

//
// Table-driven tests for ShutdownHandler
//

func TestShutdownHandler_Handle_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		callHandleTwice   bool
		trigger           func(t *testing.T, notifier *ShutdownHandlerMockNotifier, baseCancel context.CancelFunc)
		expectCancelCalls int32
	}{
		{
			name:            "signal triggers cancellation",
			callHandleTwice: false,
			trigger: func(t *testing.T, notifier *ShutdownHandlerMockNotifier, baseCancel context.CancelFunc) {
				// Signal is delivered via the channel passed to Notify.
				if notifier.notifyChan == nil {
					t.Fatalf("notifyChan must not be nil when trigger is executed")
				}
				notifier.notifyChan <- os.Interrupt
			},
			expectCancelCalls: 1,
		},
		{
			name:            "context canceled before signal",
			callHandleTwice: false,
			trigger: func(t *testing.T, notifier *ShutdownHandlerMockNotifier, baseCancel context.CancelFunc) {
				// Context is canceled externally, not via appCtxCancel.
				baseCancel()
			},
			expectCancelCalls: 0,
		},
		{
			name:            "SIGTERM triggers cancellation",
			callHandleTwice: false,
			trigger: func(t *testing.T, notifier *ShutdownHandlerMockNotifier, baseCancel context.CancelFunc) {
				if notifier.notifyChan == nil {
					t.Fatalf("notifyChan must not be nil when trigger is executed")
				}
				notifier.notifyChan <- syscall.SIGTERM
			},
			expectCancelCalls: 1,
		},
		{
			name:            "handle is idempotent",
			callHandleTwice: true,
			trigger: func(t *testing.T, notifier *ShutdownHandlerMockNotifier, baseCancel context.CancelFunc) {
				// We only care that Notify is called once, then we cancel context to let goroutine exit.
				baseCancel()
			},
			expectCancelCalls: 0,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			notifier := &ShutdownHandlerMockNotifier{}
			provider := &ShutdownHandlerMockProvider{
				signals: []os.Signal{os.Interrupt},
			}

			ctx, baseCancel := context.WithCancel(context.Background())

			var cancelCalled int32
			wrappedCancel := func() {
				atomic.AddInt32(&cancelCalled, 1)
				baseCancel()
			}

			handler := NewHandler(ctx, wrappedCancel, provider, notifier)

			// First Handle call must register subscription and start goroutine.
			handler.Handle()

			// Second call must be a no-op when callHandleTwice is true.
			if tt.callHandleTwice {
				handler.Handle()
			}

			// Notify must always be called exactly once.
			if atomic.LoadInt32(&notifier.notifyCalled) != 1 {
				t.Fatalf("Notify must be called exactly once, got %d", atomic.LoadInt32(&notifier.notifyCalled))
			}

			if notifier.notifyChan == nil {
				t.Fatalf("Notify must set notifyChan")
			}

			// Verify that signals from provider are passed to notifier.
			if len(notifier.notifySignals) != 1 || notifier.notifySignals[0] != os.Interrupt {
				t.Fatalf("Notify must be called with expected signals, got %#v", notifier.notifySignals)
			}

			// Trigger scenario-specific behavior (signal or external context cancel).
			tt.trigger(t, notifier, baseCancel)

			// Give goroutine some time to react.
			time.Sleep(30 * time.Millisecond)

			// Check how many times appCtxCancel (wrappedCancel) was called.
			if atomic.LoadInt32(&cancelCalled) != tt.expectCancelCalls {
				t.Fatalf("unexpected appCtxCancel calls: expected %d, got %d",
					tt.expectCancelCalls, atomic.LoadInt32(&cancelCalled))
			}

			// Goroutine must eventually call Stop exactly once.
			if atomic.LoadInt32(&notifier.stopCalled) != 1 {
				t.Fatalf("Stop must be called exactly once, got %d", atomic.LoadInt32(&notifier.stopCalled))
			}

			// Stop must be called with the same channel that Notify used.
			if notifier.stopChan != notifier.notifyChan {
				t.Fatalf("Stop must be called with the same channel as Notify")
			}
		})
	}
}
