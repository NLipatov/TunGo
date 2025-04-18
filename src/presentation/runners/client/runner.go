package client

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"
	"tungo/application"
)

type Runner struct {
	deps          AppDependencies
	routerFactory application.TrafficRouterFactory
}

func NewRunner(deps AppDependencies, routerFactory application.TrafficRouterFactory) *Runner {
	return &Runner{
		deps:          deps,
		routerFactory: routerFactory,
	}
}

func (r *Runner) Run(ctx context.Context) {
	defer func() {
		if err := r.deps.TunManager().DisposeTunDevices(); err != nil {
			log.Printf("error disposing tun devices on exit: %s", err)
		}
	}()

	for ctx.Err() == nil {
		err := r.runSession(ctx)
		switch {
		case err == nil, errors.Is(err, context.Canceled):
			return
		default:
			log.Printf("session error: %v, reconnecting…", err)
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func (r *Runner) runSession(parentCtx context.Context) error {
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	if err := r.deps.TunManager().DisposeTunDevices(); err != nil {
		log.Printf("error disposing tun devices: %v", err)
	}

	router, conn, tun, err := r.routerFactory.
		CreateRouter(ctx, r.deps.ConnectionFactory(), r.deps.TunManager(), r.deps.WorkerFactory())
	if err != nil {
		return fmt.Errorf("failed to create router: %s", err)
	}

	log.Printf("tunneling traffic via tun device")

	go func() {
		<-ctx.Done() //blocks until context is cancelled
		_ = conn.Close()
		_ = tun.Close()
	}()

	return router.RouteTraffic(ctx)
}
