package application

import "context"

// TunWorker does the TUN->CONN and CONN->TUN operations
type TunWorker interface {
	// HandleTun handles packets from TUN-like interface
	HandleTun(ctx context.Context) error
	// HandleTransport handles packets from transport connection
	HandleTransport(ctx context.Context) error
}
