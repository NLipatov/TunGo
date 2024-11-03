package main

import (
	"context"
	"etha-tunnel/client/forwarding/ipconfiguration"
	"etha-tunnel/client/forwarding/routing"
	"etha-tunnel/inputcommands"
	"etha-tunnel/settings/client"
	"log"
)

func main() {
	// Create a context that can be canceled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start a goroutine to listen for user input
	go inputcommands.ListenForCommand(cancel)

	// Read client configuration
	conf, err := (&client.Conf{}).Read()
	if err != nil {
		log.Fatalf("Failed to read configuration: %v", err)
	}

	// Client configuration (enabling TUN/TCP forwarding)
	ipconfiguration.Unconfigure(conf.TCPSettings)
	ipconfiguration.Unconfigure(conf.UDPSettings)

	clientRouter := routing.ClientRouter{}

	routingErr := clientRouter.Route(*conf, ctx)
	if routingErr != nil {
		log.Printf("failed to route trafic: %s", routingErr)
	}
}
