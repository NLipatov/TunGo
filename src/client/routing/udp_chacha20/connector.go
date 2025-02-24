package udp_chacha20

import (
	"context"
	"net"
	"tungo/crypto/chacha20"
	"tungo/settings"
)

type Connector struct {
	settings        settings.ConnectionSettings
	connection      Connection
	secretExchanger SecretExchanger
}

func NewConnector(settings settings.ConnectionSettings, connection Connection, secretExchanger SecretExchanger) *Connector {
	return &Connector{
		settings:        settings,
		connection:      connection,
		secretExchanger: secretExchanger,
	}
}

func (c *Connector) Connect(ctx context.Context) (*net.UDPConn, *chacha20.UdpSession, error) {
	conn, connErr := c.connection.Establish()
	if connErr != nil {
		return nil, nil, connErr
	}

	session, sessionErr := c.secretExchanger.exchange(ctx, conn)
	if sessionErr != nil {
		return nil, nil, sessionErr
	}

	return conn, session, nil
}
