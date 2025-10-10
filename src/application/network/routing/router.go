package routing

import "context"

// Router is an interface for routing traffic between client and server (manages TUN-worker and Transport-worker)
type Router interface {
	RouteTraffic(ctx context.Context) error
}
