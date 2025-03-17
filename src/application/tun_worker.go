package application

import "context"

// TunWorker does the TUN->CONN and CONN->TUN operations
type TunWorker interface {
	// HandleTun handles packets from TUN-like interface
	HandleTun(ctx context.Context, triggerReconnect context.CancelFunc) error
	// HandleConn handles packets from transport connection
	HandleConn(ctx context.Context, triggerReconnect context.CancelFunc) error
}
