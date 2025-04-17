package routing

import (
	"context"
	"fmt"
	"os"
	"sync"
	"tungo/infrastructure/platform_tun/tools_linux"
	"tungo/infrastructure/routing_layer/server_routing/routing/tcp_chacha20"
	"tungo/infrastructure/routing_layer/server_routing/routing/udp_chacha20"
	"tungo/presentation/interactive_commands"
	"tungo/settings"
)

func StartTCPRouting(ctx context.Context, tunFile *os.File, settings settings.ConnectionSettings) error {
	// Create a context that can be canceled
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Start a goroutine to listen for user input
	go interactive_commands.ListenForCommand(cancel, "server")

	// Setup server
	err := tools_linux.Configure(tunFile)
	if err != nil {
		return fmt.Errorf("failed to configure a server: %s\n", err)
	}
	defer tools_linux.Unconfigure(tunFile)

	tcpTunWorker := tcp_chacha20.NewTcpTunWorker(ctx, tunFile, settings)

	var wg sync.WaitGroup
	wg.Add(2)

	// TUN -> TCP
	go func() {
		defer wg.Done()
		_ = tcpTunWorker.HandleTun()
	}()

	// TCP -> TUN
	go func() {
		defer wg.Done()
		_ = tcpTunWorker.HandleTransport()
	}()

	wg.Wait()
	return nil
}

func StartUDPRouting(ctx context.Context, tunFile *os.File, settings settings.ConnectionSettings) error {
	// Create a context that can be canceled
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Start a goroutine to listen for user input
	go interactive_commands.ListenForCommand(cancel, "server")

	// Setup server
	err := tools_linux.Configure(tunFile)
	if err != nil {
		return fmt.Errorf("failed to configure a server: %s\n", err)
	}
	defer tools_linux.Unconfigure(tunFile)

	udpTunWorker := udp_chacha20.NewUdpTunWorker(ctx, tunFile, settings)

	var wg sync.WaitGroup
	wg.Add(2)

	// TUN -> UDP
	go func() {
		defer wg.Done()
		_ = udpTunWorker.HandleTun()
	}()

	// UDP -> TUN
	go func() {
		defer wg.Done()
		_ = udpTunWorker.HandleTransport()
	}()

	wg.Wait()
	return nil
}
