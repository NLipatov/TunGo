package application

import "context"

// TrafficRouter is an interface for routing traffic between client and server
type TrafficRouter interface {
	RouteTraffic(ctx context.Context) error
}
