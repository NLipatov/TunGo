package presentation

import (
	"context"
	"log"
	"time"
	"tungo/infrastructure/tun_device"
	"tungo/presentation/client_routing"
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

	// Setup platform tun configurator
	tunDevConfigurator, tunDevConfiguratorErr := tun_device.NewTunDeviceConfigurator(*conf)
	if tunDevConfiguratorErr != nil {
		log.Fatalf("failed to configure tun: %s", tunDevConfiguratorErr)
	}

	for ctx.Err() == nil {

		// Clear all existing TunGo tun devices
		_ = tunDevConfigurator.DisposeTunDevices()

		// Build router. (udp or tcp based on client's conf.json file)
		routerBuilder := client_routing.NewRouterBuilder()
		router, routerErr := routerBuilder.Build(ctx, *conf, tunDevConfigurator)
		if routerErr != nil {
			log.Printf("failed to create router: %s", routerErr)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// Start routing traffic using router
		routeTrafficErr := router.RouteTraffic(ctx)
		if routeTrafficErr != nil {
			log.Printf("routing err: %s", routeTrafficErr)
		}
	}

	// Remove TUN-device before exiting
	_ = tunDevConfigurator.DisposeTunDevices()
}
