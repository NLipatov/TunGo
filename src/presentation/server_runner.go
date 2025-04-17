package presentation

import (
	"context"
	"log"
	"time"
)

type ServerRunner struct {
	deps ServerAppDependencies
}

func NewServerRunner(deps ClientAppDependencies) *ServerRunner {
	return &ServerRunner{}
}

func (r *ServerRunner) Run(ctx context.Context) {
	if err := r.deps.TunManager().DisposeTunDevices(); err != nil {
		log.Printf("error disposing tun devices: %s", err)
	}

	router, conn, tun, err := r.routerFactory.
		CreateRouter(ctx, r.deps.ConnectionFactory(), r.deps.TunManager(), r.deps.WorkerFactory())
	if err != nil {
		log.Printf("failed to create router: %s", err)
		time.Sleep(500 * time.Millisecond)
		continue
	}
}
