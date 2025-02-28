package routing

import (
	"context"
	"fmt"
	"os"
	"sync"
	"tungo/cmd"
	"tungo/server"
	"tungo/server/serveripconf"
	"tungo/settings"
)

func StartTCPRouting(tunFile *os.File, settings settings.ConnectionSettings) error {
	// Create a context that can be canceled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start a goroutine to listen for user input
	go cmd.ListenForCommand(cancel, "server")

	// Setup server
	err := serveripconf.Configure(tunFile)
	if err != nil {
		return fmt.Errorf("failed to configure a server: %s\n", err)
	}
	defer serveripconf.Unconfigure(tunFile)

	// Map to keep track of connected clients
	var extToLocalIp sync.Map   // external ip to local ip map
	var extIpToSession sync.Map // external ip to session map

	tcpTunWorker := server.NewTcpTunWorker()

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

func StartUDPRouting(tunFile *os.File, settings settings.ConnectionSettings) error {
	// Create a context that can be canceled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start a goroutine to listen for user input
	go cmd.ListenForCommand(cancel, "server")

	// Setup server
	err := serveripconf.Configure(tunFile)
	if err != nil {
		return fmt.Errorf("failed to configure a server: %s\n", err)
	}
	defer serveripconf.Unconfigure(tunFile)

	// Map to keep track of connected clients
	var intIPToUDPClient sync.Map // external ip to local ip map
	var intIPToSession sync.Map   // external ip to session map

	udpTunWorker := server.NewUdpTunWorker(ctx, tunFile, settings, &intIPToUDPClient, &intIPToSession)

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
