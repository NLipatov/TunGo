package routing

import (
	"context"
	"fmt"
	"os"
	"sync"
	"tungo/presentation/interactive_commands"
	"tungo/presentation/server_routing"
	"tungo/settings"
)

func StartTCPRouting(ctx context.Context, tunFile *os.File, settings settings.ConnectionSettings) error {
	// Create a context that can be canceled
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Start a goroutine to listen for user input
	go interactive_commands.ListenForCommand(cancel, "server")

	// Setup server
	err := server_routing.Configure(tunFile)
	if err != nil {
		return fmt.Errorf("failed to configure a server: %s\n", err)
	}
	defer server_routing.Unconfigure(tunFile)

	// Map to keep track of connected clients
	var extToLocalIp sync.Map   // external ip to local ip map
	var extIpToSession sync.Map // external ip to session map

	tcpTunWorker := server_routing.NewTcpTunWorker()

	var wg sync.WaitGroup
	wg.Add(2)

	// TUN -> TCP
	go func() {
		defer wg.Done()
		tcpTunWorker.TunToTCP(tunFile, &extToLocalIp, &extIpToSession, ctx)
	}()

	// TCP -> TUN
	go func() {
		defer wg.Done()
		tcpTunWorker.TCPToTun(settings, tunFile, &extToLocalIp, &extIpToSession, ctx)
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
	err := server_routing.Configure(tunFile)
	if err != nil {
		return fmt.Errorf("failed to configure a server: %s\n", err)
	}
	defer server_routing.Unconfigure(tunFile)

	udpTunWorker := server_routing.NewUdpTunWorker(ctx, tunFile, settings)

	var wg sync.WaitGroup
	wg.Add(2)

	// TUN -> UDP
	go func() {
		defer wg.Done()
		udpTunWorker.TunToUDP()
	}()

	// UDP -> TUN
	go func() {
		defer wg.Done()
		udpTunWorker.UDPToTun()
	}()

	wg.Wait()
	return nil
}
