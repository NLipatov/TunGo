package routing

import "context"

type (
	// TunWorker does the TUN->CONN and CONN->TUN operations
	TunWorker interface {
		// HandleTun handles packets from TUN-like interface
		HandleTun(ctx context.Context, triggerReconnect context.CancelFunc) error
		// HandleConn handles packets from transport connection
		HandleConn(ctx context.Context, triggerReconnect context.CancelFunc) error
	}

	// TrafficRouter is an interface for routing traffic between client and server
	TrafficRouter interface {
		RouteTraffic(ctx context.Context) error
	}
)
