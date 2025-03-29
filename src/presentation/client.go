package presentation

import (
	"context"
	"log"
	"time"
	"tungo/infrastructure/tun_device"
	"tungo/presentation/client_routing"
	"tungo/presentation/interactive_commands"
	"tungo/settings/client_configuration"
)

func StartClient() {
	// Create a context that can be canceled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start a goroutine to listen for user input
	go interactive_commands.ListenForCommand(cancel, "client")

	// Read client configuration
	configurationManager := client_configuration.NewManager()
	clientConf, clientConfErr := configurationManager.Configuration()
	if clientConfErr != nil {
		log.Fatalf("failed to read client configuration: %s", clientConfErr)
	}

	// Setup platform tun configurator
	tunDevConfigurator, tunDevConfiguratorErr := tun_device.NewAbstractTunFactory(*clientConf)
	if tunDevConfiguratorErr != nil {
		log.Fatalf("failed to configure tun: %s", tunDevConfiguratorErr)
	}

	for ctx.Err() == nil {

		// Clear all existing TunGo tun devices
		_ = tunDevConfigurator.DisposeTunDevices()

		// Build router. (udp or tcp based on client's conf.json file)
		routerBuilder := client_routing.NewRouterBuilder()
		router, routerErr := routerBuilder.Build(ctx, *clientConf, tunDevConfigurator)
		if routerErr != nil {
			log.Printf("failed to create router: %s", routerErr)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		log.Printf("tunneling traffic via tun device")

		// Start routing traffic using router
		router.RouteTraffic(ctx)
	}

	// Remove TUN-device before exiting
	_ = tunDevConfigurator.DisposeTunDevices()
}
