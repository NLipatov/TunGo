package network

import (
	"tungo/application/network/connection"
)

type SecureSession interface {
	Establish() (connection.Transport, connection.Crypto, error)
}

type DefaultSecureSession struct {
	transport connection.Transport
	secret    Secret
}

func NewDefaultSecureSession(transport connection.Transport, secret Secret) *DefaultSecureSession {
	return &DefaultSecureSession{
		transport: transport,
		secret:    secret,
	}
}

func (c *DefaultSecureSession) Establish() (connection.Transport, connection.Crypto, error) {
	crypto, err := c.secret.Exchange(c.transport)
	if err != nil {
		return nil, nil, err
	}

	return c.transport, crypto, nil
}
