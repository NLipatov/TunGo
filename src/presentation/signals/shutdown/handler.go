package shutdown

import (
	"context"
	"log"
	"os"
	"sync"
	palSignal "tungo/infrastructure/PAL/signal"
	"tungo/presentation/signals"
)

type Handler struct {
	// appCtx is application context.
	// If this context is cancelled - handler must stop its job and return.
	appCtx context.Context
	// appCtxCancel - cancellation func that must be used to cancel appCtx.
	appCtxCancel context.CancelFunc
	// signalChan - channel of signals that handler is supposed to handle (shutdown signals in this case)
	signalChan chan os.Signal
	once       sync.Once
	// signalProvider is used to provide shutdown signal set for current platform.
	signalProvider palSignal.Provider
	// notifier used to subscribe to OS Signal and to unsubscribe from it
	notifier signals.Notifier
}

func NewHandler(
	appCtx context.Context,
	appCtxCancel context.CancelFunc,
	signalProvider palSignal.Provider,
	notifier signals.Notifier,
) signals.Handler {
	return &Handler{
		appCtx:       appCtx,
		appCtxCancel: appCtxCancel,
		// Note: 1-sized buffer used as os/signal uses non-blocking sends and may drop signals if unbuffered.
		signalChan:     make(chan os.Signal, 1),
		signalProvider: signalProvider,
		notifier:       notifier,
	}
}

func (h *Handler) Handle() {
	h.once.Do(func() {
		h.listenAndHandleShutdownSignals()
	})
}

func (h *Handler) listenAndHandleShutdownSignals() {
	h.subscribe()
	go func() {
		defer func() {
			h.unsubscribe()
		}()
		select {
		case <-h.signalChan:
			log.Printf("Shutdown signal received. Shutting down...")
			h.appCtxCancel()
		case <-h.appCtx.Done():
		}
	}()
}

func (h *Handler) subscribe() {
	h.notifier.Notify(h.signalChan, h.signalProvider.ShutdownSignals()...)
}

func (h *Handler) unsubscribe() {
	h.notifier.Stop(h.signalChan)
}
