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

	// Read client configuration
	conf, err := (&client.Conf{}).Read()
	if err != nil {
		log.Fatalf("Failed to read configuration: %v", err)
	}

	switch conf.Protocol {
	case 0:
		if err := ipconfiguration.Configure(conf.TCPSettings); err != nil {
			log.Fatalf("Failed to configure client: %v", err)
		}
		// Open the TUN interface
		tunFile, err := network.OpenTunByName(conf.TCPSettings.InterfaceName)
		if err != nil {
			log.Fatalf("Failed to open TUN interface: %v", err)
		}
		defer tunFile.Close()

		err = routing.StartTCPRouting(conf.TCPSettings, tunFile, &ctx)
		if err != nil {
			log.Fatalf("failed to route trafic: %s", err)
		}
	case 1:
		if err := ipconfiguration.Configure(conf.UDPSettings); err != nil {
			log.Fatalf("Failed to configure client: %v", err)
		}
		// Open the TUN interface
		tunFile, err := network.OpenTunByName(conf.UDPSettings.InterfaceName)
		if err != nil {
			log.Fatalf("Failed to open TUN interface: %v", err)
		}
		defer tunFile.Close()

		err = routing.StartUDPRouting(conf.UDPSettings, tunFile, &ctx)
		if err != nil {
			log.Fatalf("failed to route trafic: %s", err)
		}
	}
}
