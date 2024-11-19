package routing

import (
	"context"
)

// Router defines the interface for a protocol-specific router.
type Router interface {
	ForwardTraffic(ctx context.Context) error
}
