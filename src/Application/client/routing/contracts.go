package routing

import "context"

type (
	// TunWorker does the TUN->CONN and CONN->TUN operations
	TunWorker interface {
		// HandlePacketsFromTun handles packets from TUN-like interface
		HandlePacketsFromTun(ctx context.Context, triggerReconnect context.CancelFunc) error
		// HandlePacketsFromConn handles packets from transport connection
		HandlePacketsFromConn(ctx context.Context, triggerReconnect context.CancelFunc) error
	}

	// TrafficRouter is an interface for routing traffic between client and server
	TrafficRouter interface {
		RouteTraffic(ctx context.Context) error
	}
)
