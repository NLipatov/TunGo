package runtime

import (
	"context"
	"fmt"
	"sync"
	runtimeClient "tungo/runtime/internal/client"
	runtimeServer "tungo/runtime/internal/server"
)

func Start(ctx context.Context, mode Mode) (Session, error) {
	switch mode {
	case ModeServer:
		return startServer(ctx)
	case ModeClient:
		return startClient(ctx)
	default:
		return nil, fmt.Errorf("invalid runtime mode: %v", mode)
	}
}

func startClient(ctx context.Context) (Session, error) {
	clientRuntime, err := runtimeClient.NewRuntime()
	if err != nil {
		return nil, err
	}

	sessionCtx, cancel := context.WithCancel(ctx)
	readyCh := make(chan struct{})
	session := newRunningSession(
		sessionCtx,
		readyCh,
		cancel,
	)

	go func() {
		session.finish(clientRuntime.Run(sessionCtx, readyCh))
	}()
	return session, nil
}

func startServer(ctx context.Context) (Session, error) {
	resolver, manager, err := runtimeServer.NewDefaultConfiguration()
	if err != nil {
		return nil, err
	}
	serverRuntime, err := runtimeServer.NewRuntime(ctx, resolver, manager)
	if err != nil {
		return nil, err
	}

	sessionCtx, cancel := context.WithCancel(ctx)
	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			cancel()
			serverRuntime.Stop()
		})
	}

	session := newRunningSession(
		sessionCtx,
		closedReadyCh(),
		stop,
	)

	go func() {
		defer stop()
		session.finish(serverRuntime.Run(sessionCtx))
	}()
	return session, nil
}

func closedReadyCh() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}
