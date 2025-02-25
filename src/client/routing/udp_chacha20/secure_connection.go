package udp_chacha20

import (
	"net"
	"tungo/crypto/chacha20"
)

type SecureConnection struct {
	connection Connection
	secret     Secret
}

func NewSecureConnection(connection Connection, secret Secret) *SecureConnection {
	return &SecureConnection{
		connection: connection,
		secret:     secret,
	}
}

func (c *SecureConnection) Establish() (*net.UDPConn, *chacha20.UdpSession, error) {
	conn, connErr := c.connection.Establish()
	if connErr != nil {
		return nil, nil, connErr
	}

	session, sessionErr := c.secret.Exchange(conn)
	if sessionErr != nil {
		return nil, nil, sessionErr
	}

	return conn, session, nil
}
