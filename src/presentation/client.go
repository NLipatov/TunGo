package presentation

import (
	"context"
	"log"
	"tungo/presentation/interactive_commands"
	"tungo/settings/client_configuration"
)

func StartClient() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go interactive_commands.ListenForCommand(cancel, "client")

	deps := NewClientDependencies(client_configuration.NewManager())
	depsErr := deps.Initialize()
	if depsErr != nil {
		log.Fatalf("init error: %s", depsErr)
	}

	runner := NewClientRunner(deps)
	runner.Run(ctx)
}
