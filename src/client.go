package main

import (
	"context"
	"etha-tunnel/client/forwarding/ipconfiguration"
	"etha-tunnel/client/forwarding/routing"
	"etha-tunnel/inputcommands"
	"etha-tunnel/network"
	"etha-tunnel/settings/client"
	"log"
)

func main() {
	// Create a context that can be canceled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start a goroutine to listen for user input
	go inputcommands.ListenForCommand(cancel)

	// Client configuration (enabling TUN/TCP forwarding)
	ipconfiguration.Unconfigure()
	defer ipconfiguration.Unconfigure()
	if err := ipconfiguration.Configure(); err != nil {
		log.Fatalf("Failed to configure client: %v", err)
	}

	// Read client configuration
	conf, err := (&client.Conf{}).Read()
	if err != nil {
		log.Fatalf("Failed to read configuration: %v", err)
	}

	// Open the TUN interface
	tunFile, err := network.OpenTunByName(conf.TCPSettings.InterfaceName)
	if err != nil {
		log.Fatalf("Failed to open TUN interface: %v", err)
	}
	defer tunFile.Close()

	switch conf.Protocol {
	case 0:
		err = routing.StartTCPRouting(conf, tunFile, &ctx)
		if err != nil {
			log.Fatalf("failed to route trafic: %s", err)
		}
	}
}
