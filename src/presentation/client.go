package presentation

import (
	"context"
	"log"
	"tungo/presentation/client_routing/routing"
	"tungo/presentation/interactive_commands"
	"tungo/settings/client"
)

func StartClient() {
	// Create a context that can be canceled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start a goroutine to listen for user input
	go interactive_commands.ListenForCommand(cancel, "client")

	// Read client configuration
	conf, err := (&client.Conf{}).Read()
	if err != nil {
		log.Fatalf("failed to read configuration: %v", err)
	}

	routerBuilder := routing.NewRouterBuilder()
	router, routerErr := routerBuilder.Build(ctx, *conf)
	if routerErr != nil {
		log.Fatalf("failed to create router: %s", routerErr)
	}

	routeTrafficErr := router.RouteTraffic(ctx)
	if routeTrafficErr != nil {
		log.Printf("routing err: %s", routeTrafficErr)
	}
}
