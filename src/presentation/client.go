package presentation

import (
	"context"
	"log"
	"tungo/settings/client_configuration"
)

func StartClient(ctx context.Context) {
	deps := NewClientDependencies(client_configuration.NewManager())
	depsErr := deps.Initialize()
	if depsErr != nil {
		log.Fatalf("init error: %s", depsErr)
	}

	runner := NewClientRunner(deps)
	runner.Run(ctx)
}
