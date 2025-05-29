package tcp_connection

import (
	"tungo/application"
)

type SecureSession interface {
	Establish() (application.ConnectionAdapter, application.CryptographyService, error)
}

type DefaultSecureSession struct {
	connection application.Connection
	secret     Secret
}

func NewDefaultSecureSession(connection application.Connection, secret Secret) *DefaultSecureSession {
	return &DefaultSecureSession{
		connection: connection,
		secret:     secret,
	}
}

func (c *DefaultSecureSession) Establish() (application.ConnectionAdapter, application.CryptographyService, error) {
	conn, connErr := c.connection.Establish()
	if connErr != nil {
		return nil, nil, connErr
	}

	cryptographyService, cryptographyServiceErr := c.secret.Exchange(conn)
	if cryptographyServiceErr != nil {
		return nil, nil, cryptographyServiceErr
	}

	return conn, cryptographyService, nil
}
