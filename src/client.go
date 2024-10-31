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

	// Read client configuration
	conf, err := (&client.Conf{}).Read()
	if err != nil {
		log.Fatalf("Failed to read configuration: %v", err)
	}

	// Client configuration (enabling TUN/TCP forwarding)
	ipconfiguration.Unconfigure(conf.TCPSettings)
	ipconfiguration.Unconfigure(conf.UDPSettings)

	switch conf.Protocol {
	case 0:
		// Configure client
		if tcpConfigurationErr := ipconfiguration.Configure(conf.TCPSettings); tcpConfigurationErr != nil {
			log.Fatalf("Failed to configure client: %v", tcpConfigurationErr)
		}
		defer ipconfiguration.Unconfigure(conf.TCPSettings)

		// Open the TUN interface
		tunFile, openTunErr := network.OpenTunByName(conf.TCPSettings.InterfaceName)
		if openTunErr != nil {
			log.Fatalf("Failed to open TUN interface: %v", openTunErr)
		}
		defer tunFile.Close()

		routingErr := routing.StartTCPRouting(conf.TCPSettings, tunFile, ctx)
		if ctx.Err() != nil {
			return
		}
		if routingErr != nil {
			log.Printf("failed to route trafic: %s", routingErr)
		}
	case 1:
		// Configure client
		if udpConfigurationErr := ipconfiguration.Configure(conf.UDPSettings); udpConfigurationErr != nil {
			log.Fatalf("Failed to configure client: %v", udpConfigurationErr)
		}
		defer ipconfiguration.Unconfigure(conf.UDPSettings)

		// Open the TUN interface
		tunFile, openTunErr := network.OpenTunByName(conf.UDPSettings.InterfaceName)
		if openTunErr != nil {
			log.Fatalf("Failed to open TUN interface: %v", openTunErr)
		}
		defer tunFile.Close()

		routingErr := routing.StartUDPRouting(conf.UDPSettings, tunFile, &ctx)
		if routingErr != nil {
			log.Fatalf("failed to route trafic: %s", routingErr)
		}
	}
}
