package runtime

import (
	"context"
	"fmt"
	runtimeStart "tungo/runtime/internal/start"
)

func Start(ctx context.Context, mode Mode) (Session, error) {
	switch mode {
	case ModeServer:
		return runtimeStart.Server(ctx)
	case ModeClient:
		return runtimeStart.Client(ctx)
	default:
		return nil, fmt.Errorf("invalid runtime mode: %v", mode)
	}
}
