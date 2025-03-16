package client_routing

import (
	"context"
	"fmt"
	"log"
	"math"
	"net"
	"time"
	"tungo/application"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/presentation/client_routing/routing/tcp_chacha20"
	"tungo/presentation/client_routing/routing/tcp_chacha20/tcp_connection"
	"tungo/presentation/client_routing/routing/udp_chacha20"
	"tungo/presentation/client_routing/routing/udp_chacha20/udp_connection"
	"tungo/settings"
	"tungo/settings/client"
)

type RouterBuilder struct {
}

func NewRouterBuilder() RouterBuilder {
	return RouterBuilder{}
}

func (u *RouterBuilder) Build(
	ctx context.Context, conf client.Conf, tunDevConfigurator application.PlatformTunConfigurator,
) (application.TrafficRouter, error) {
	tun, tunErr := tunDevConfigurator.CreateTunDevice()
	if tunErr != nil {
		log.Printf("failed to create tun: %s", tunErr)
	}

	switch conf.Protocol {
	case settings.UDP:
		conn, cryptographyService, connErr := u.udpConn(ctx, conf.UDPSettings)
		if connErr != nil {
			return nil, connErr
		}
		return udp_chacha20.NewUDPRouter(conn, tun, cryptographyService), nil
	case settings.TCP:
		conn, cryptographyService, connErr := u.tcpConn(ctx, conf.TCPSettings)
		if connErr != nil {
			return nil, connErr
		}
		return tcp_chacha20.NewTCPRouter(conn, tun, cryptographyService), nil
	default:
		return nil, fmt.Errorf("unsupported protocol")
	}
}

func (u *RouterBuilder) tcpConn(ctx context.Context, settings settings.ConnectionSettings) (*net.Conn, application.CryptographyService, error) {
	//setup ctx deadline
	deadline := time.Now().Add(time.Duration(math.Max(float64(settings.DialTimeoutMs), 5000)) * time.Millisecond)
	handshakeCtx, handshakeCtxCancel := context.WithDeadline(ctx, deadline)
	defer handshakeCtxCancel()

	//connect to server and exchange secret
	secret := tcp_connection.NewDefaultSecret(settings, chacha20.NewHandshake())
	cancellableSecret := tcp_connection.NewSecretWithDeadline(handshakeCtx, secret)

	session := tcp_connection.NewDefaultSecureSession(tcp_connection.NewDefaultConnection(settings), cancellableSecret)
	cancellableSession := tcp_connection.NewSecureSessionWithDeadline(handshakeCtx, session)
	conn, tcpSession, err := cancellableSession.Establish()
	if err != nil {
		return nil, nil, err
	}

	return conn, tcpSession, nil
}

func (u *RouterBuilder) udpConn(ctx context.Context, settings settings.ConnectionSettings) (*net.UDPConn, application.CryptographyService, error) {
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
