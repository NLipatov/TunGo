package routing

import (
	"context"
	"os"
	"sync"
	"tungo/Application/server"
	"tungo/Domain/settings"
)

func StartTCPRouting(ctx context.Context, tunFile *os.File, settings settings.ConnectionSettings) error {
	// Map to keep track of connected clients
	var extToLocalIp sync.Map   // external ip to local ip map
	var extIpToSession sync.Map // external ip to session map

	var wg sync.WaitGroup
	wg.Add(2)

	// TUN -> TCP
	go func() {
		defer wg.Done()
		server.TunToTCP(tunFile, &extToLocalIp, &extIpToSession, ctx)
	}()

	// TCP -> TUN
	go func() {
		defer wg.Done()
		server.TCPToTun(settings, tunFile, &extToLocalIp, &extIpToSession, ctx)
	}()

	wg.Wait()
	return nil
}

func StartUDPRouting(ctx context.Context, tunFile *os.File, settings settings.ConnectionSettings) error {
	// Map to keep track of connected clients
	var intIPToUDPClient sync.Map // external ip to local ip map
	var intIPToSession sync.Map   // external ip to session map

	var wg sync.WaitGroup
	wg.Add(2)

	// TUN -> UDP
	go func() {
		defer wg.Done()
		server.TunToUDP(tunFile, &intIPToUDPClient, &intIPToSession, ctx)
	}()

	// UDP -> TUN
	go func() {
		defer wg.Done()
		server.UDPToTun(settings, tunFile, &intIPToUDPClient, &intIPToSession, ctx)
	}()

	wg.Wait()
	return nil
}
