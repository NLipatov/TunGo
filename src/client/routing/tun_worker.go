package routing

import (
	"context"
)

// TunWorker does the TUN->CONN and CONN->TUN operations
type TunWorker interface {
	// HandlePacketsFromTun handles packets from TUN-like interface
	HandlePacketsFromTun(ctx context.Context, triggerReconnect context.CancelFunc) error
	// HandlePacketsFromConn handles packets from transport connection
	HandlePacketsFromConn(ctx context.Context, triggerReconnect context.CancelFunc) error
}
