package routing

import (
	"context"
	"fmt"
	"os"
	"sync"
	"tungo/Application/boundary"
	"tungo/Application/server"
	"tungo/Application/server/routing/tun_tcp_chacha20"
	"tungo/Domain/settings"
)

func StartTCPRouting(ctx context.Context, tunFile boundary.TunAdapter, settings settings.ConnectionSettings) error {
	// Map to keep track of connected clients
	var extToLocalIp sync.Map   // external ip to local ip map
	var extIpToSession sync.Map // external ip to session map

	router, err := tun_tcp_chacha20.NewTCPRouter().
		UseContext(ctx).
		UseLocalIPMap(&extToLocalIp).
		UseLocalIPToSessionMap(&extIpToSession).
		UseTun(tunFile).
		UseSettings(settings).
		Build()

	if err != nil {
		return fmt.Errorf("failed to build router: %s", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// TUN -> TCP
	go func() {
		defer wg.Done()
		router.TunToTCP()
	}()

	// TCP -> TUN
	go func() {
		defer wg.Done()
		router.TCPToTun()
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
