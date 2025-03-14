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

	routerFactory := routing.NewRouterFactory()
	router, factoryErr := routerFactory.CreateRouter(*conf)
	if factoryErr != nil {
		log.Fatalf("failed to create a %v router: %s", conf.Protocol, factoryErr)
	}

	routingErr := router.RouteTraffic(ctx)
	if routingErr != nil {
		log.Printf("failed to route trafic: %s", routingErr)
	}
}
