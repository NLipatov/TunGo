package start

import (
	"context"
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
	session := lifecycle.New(cancel)

	go func() {
		defer session.Stop()
		session.Finish(clientRuntime.Run(sessionCtx, session.MarkReady))
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
	stop := func() {
		cancel()
		serverRuntime.Stop()
	}

	session := lifecycle.New(stop)
	session.MarkReady()

	go func() {
		defer session.Stop()
		session.Finish(serverRuntime.Run(sessionCtx))
	}()
	return session, nil
}
