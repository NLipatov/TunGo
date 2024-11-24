package presentation

import (
	"context"
	"log"
	"tungo/client/routing"
	"tungo/cmd"
	"tungo/settings/client"
)

func StartClient() {
	// Create a context that can be canceled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start a goroutine to listen for user input
	go cmd.ListenForCommand(cancel, "client")

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

	routingErr := router.ForwardTraffic(ctx)
	if routingErr != nil {
		log.Printf("failed to route trafic: %s", routingErr)
	}
}
