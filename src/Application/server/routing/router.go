package routing

import (
	"context"
	"fmt"
	"os"
	"sync"
	"tungo/Application/boundary"
	"tungo/Application/server/routing/tun_tcp_chacha20"
	"tungo/Application/server/routing/tun_udp_chacha20"
	"tungo/Domain/settings"
)

func StartTCPRouting(ctx context.Context, tunFile boundary.TunAdapter, settings settings.ConnectionSettings) error {
	router, err := tun_tcp_chacha20.NewTCPRouter().
		UseContext(ctx).
		UseLocalIPMap(&sync.Map{}).
		UseLocalIPToSessionMap(&sync.Map{}).
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
	router, err := tun_udp_chacha20.NewUDPRouter().
		UseSettings(settings).
		UseLocalIPToSessionMap(&sync.Map{}).
		UseIntIPToUDPClientAddr(&sync.Map{}).
		UseClientAddrToInternalIP(&sync.Map{}).
		UseTun(tunFile).
		UseContext(ctx).
		Build()

	if err != nil {
		return fmt.Errorf("failed to build router: %s", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// TUN -> UDP
	go func() {
		defer wg.Done()
		router.TunToUDP()
	}()

	// UDP -> TUN
	go func() {
		defer wg.Done()
		router.UDPToTun()
	}()

	wg.Wait()
	return nil
}
