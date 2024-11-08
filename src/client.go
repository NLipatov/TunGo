package main

import (
	"context"
	"log"
	"tungo/client/forwarding/routing"
	"tungo/cmd"
	"tungo/settings/client"
)

func main() {
	// Create a context that can be canceled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start a goroutine to listen for user input
	go cmd.ListenForCommand(cancel)

	// Read client configuration
	conf, err := (&client.Conf{}).Read()
	if err != nil {
		log.Fatalf("failed to read configuration: %v", err)
	}

	clientRouter := routing.ClientRouter{}

	routingErr := clientRouter.Route(*conf, ctx)
	if routingErr != nil {
		log.Printf("failed to route trafic: %s", routingErr)
	}
}
