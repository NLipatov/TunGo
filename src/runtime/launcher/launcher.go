package launcher

import (
	"context"
	"errors"
	"fmt"
	appRuntime "tungo/runtime"
	"tungo/runtime/launcher/internal/client"
	"tungo/runtime/launcher/internal/server"
)

type Launcher struct{}

func New() Launcher {
	return Launcher{}
}

func Run(ctx context.Context, mode appRuntime.Mode) error {
	switch mode {
	case appRuntime.ModeServer:
		return runtimeErrOrNil(ctx, server.Run(ctx))
	case appRuntime.ModeClient:
		return runtimeErrOrNil(ctx, client.Run(ctx))
	default:
		return fmt.Errorf("invalid runtime mode: %v", mode)
	}
}

func runtimeErrOrNil(ctx context.Context, err error) error {
	if err != nil && ctx.Err() == nil &&
		!errors.Is(err, context.Canceled) &&
		!errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return nil
}

func (Launcher) Start(ctx context.Context, mode appRuntime.Mode) (appRuntime.Session, error) {
	switch mode {
	case appRuntime.ModeServer:
		return server.Start(ctx)
	case appRuntime.ModeClient:
		return client.Start(ctx)
	default:
		return nil, fmt.Errorf("invalid runtime mode: %v", mode)
	}
}
