package presentation

import (
	"context"
	"log"
	"math"
	"net"
	"time"
	"tungo/application"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/tun_device"
	"tungo/presentation/client_routing/routing/udp_chacha20"
	"tungo/presentation/client_routing/routing/udp_chacha20/udp_connection"
	"tungo/presentation/interactive_commands"
	"tungo/settings"
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

	switch conf.Protocol {
	case settings.UDP:
		routeUdp(ctx, *conf)
	default:
		panic("unsupported protocol")
	}
}

func routeUdp(ctx context.Context, conf client.Conf) {
	for {
		if ctx.Err() != nil {
			log.Printf("ctx err: %s", ctx.Err())
			return
		}

		conn, cryptographyService, connErr := udpConn(ctx, conf.UDPSettings)
		if connErr != nil {
			log.Printf("failed to establish conn with %s", conf.UDPSettings.ConnectionIP)
		}

		tunDevice, tunDeviceErr := tun_device.NewTunDevice(conf)
		if tunDeviceErr != nil {
			time.Sleep(time.Millisecond * 1000)
			continue
		}

		router := udp_chacha20.NewUDPRouter(conn, tunDevice, cryptographyService)

		router.RouteTraffic(ctx)
	}
}

func udpConn(ctx context.Context, settings settings.ConnectionSettings) (*net.UDPConn, application.CryptographyService, error) {
	//setup ctx deadline
	deadline := time.Now().Add(time.Duration(math.Max(float64(settings.DialTimeoutMs), 5000)) * time.Millisecond)
	handshakeCtx, handshakeCtxCancel := context.WithDeadline(ctx, deadline)
	defer handshakeCtxCancel()

	//connect to server and exchange secret
	secret := udp_connection.NewDefaultSecret(settings, chacha20.NewHandshake())
	cancellableSecret := udp_connection.NewSecretWithDeadline(handshakeCtx, secret)

	session := udp_connection.NewDefaultSecureSession(udp_connection.NewConnection(settings), cancellableSecret)
	cancellableSession := udp_connection.NewSecureSessionWithDeadline(handshakeCtx, session)
	return cancellableSession.Establish()
}
