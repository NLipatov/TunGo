package client_routing

import (
	"context"
	"fmt"
	"math"
	"net"
	"time"
	"tungo/application"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/presentation/client_routing/routing/tcp_chacha20/tcp_connection"
	"tungo/presentation/client_routing/routing/udp_chacha20/udp_connection"
	"tungo/settings"
)

type ConnectionFactory struct {
}

func NewConnectionFactory() *ConnectionFactory {
	return &ConnectionFactory{}
}

func (f *ConnectionFactory) EstablishConnection(
	ctx context.Context, s settings.ConnectionSettings,
) (net.Conn, application.CryptographyService, error) {
	deadline := time.Now().Add(time.Duration(math.Max(float64(s.DialTimeoutMs), 5000)) * time.Millisecond)
	handshakeCtx, handshakeCtxCancel := context.WithDeadline(ctx, deadline)
	defer handshakeCtxCancel()

	switch s.Protocol {
	case settings.UDP:
		//connect to server and exchange secret
		secret := udp_connection.NewDefaultSecret(s, chacha20.NewHandshake())
		cancellableSecret := udp_connection.NewSecretWithDeadline(handshakeCtx, secret)

		session := udp_connection.NewDefaultSecureSession(udp_connection.NewConnection(s), cancellableSecret)
		cancellableSession := udp_connection.NewSecureSessionWithDeadline(handshakeCtx, session)
		return cancellableSession.Establish()
	case settings.TCP:
		//connect to server and exchange secret
		secret := tcp_connection.NewDefaultSecret(s, chacha20.NewHandshake())
		cancellableSecret := tcp_connection.NewSecretWithDeadline(handshakeCtx, secret)

		session := tcp_connection.NewDefaultSecureSession(tcp_connection.NewDefaultConnection(s), cancellableSecret)
		cancellableSession := tcp_connection.NewSecureSessionWithDeadline(handshakeCtx, session)
		return cancellableSession.Establish()
	default:
		return nil, nil, fmt.Errorf("unsupported protocol: %v", s.Protocol)
	}
}
