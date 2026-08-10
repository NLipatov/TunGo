package shutdown

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
)

func Handle(ctx context.Context, cancel context.CancelFunc) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, shutdownSignals[:]...)

	go func() {
		defer signal.Stop(signals)
		select {
		case <-signals:
			slog.Info("shutdown signal received")
			cancel()
		case <-ctx.Done():
		}
	}()
}
