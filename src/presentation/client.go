package presentation

import (
	"context"
	"log"
	"tungo/application"
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

	tunDevConfigurator, tunDevConfiguratorErr := tun_device.NewTunDeviceConfigurator(*conf)
	if tunDevConfiguratorErr != nil {
		log.Fatalf("failed to configure tun: %s", tunDevConfiguratorErr)
	}
	_ = tunDevConfigurator.DisposeTunDevices()
	defer func(tunDevConfigurator application.PlatformTunConfigurator) {
		_ = tunDevConfigurator.DisposeTunDevices()
	}(tunDevConfigurator)

	routerBuilder := client_routing.NewRouterBuilder()
	router, routerErr := routerBuilder.Build(ctx, *conf, tunDevConfigurator)
	if routerErr != nil {
		log.Fatalf("failed to create router: %s", routerErr)
	}

	routeTrafficErr := router.RouteTraffic(ctx)
	if routeTrafficErr != nil {
		log.Printf("routing err: %s", routeTrafficErr)
	}
}
