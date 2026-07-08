package start

import (
	"context"
	"sync"
	runtimeClient "tungo/runtime/internal/client"
	"tungo/runtime/internal/lifecycle"
	runtimeServer "tungo/runtime/internal/server"
)

func Client(ctx context.Context) (*lifecycle.Session, error) {
	clientRuntime, err := runtimeClient.NewRuntime()
	if err != nil {
		return nil, err
	}

	sessionCtx, cancel := context.WithCancel(ctx)
	readyCh := make(chan struct{})
	session := lifecycle.New(
		sessionCtx,
		readyCh,
		cancel,
	)

	go func() {
		session.Finish(clientRuntime.Run(sessionCtx, readyCh))
	}()
	return session, nil
}

func Server(ctx context.Context) (*lifecycle.Session, error) {
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

	session := lifecycle.New(
		sessionCtx,
		lifecycle.ClosedReadyCh(),
		stop,
	)

	go func() {
		defer stop()
		session.Finish(serverRuntime.Run(sessionCtx))
	}()
	return session, nil
}
