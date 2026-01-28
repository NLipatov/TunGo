package network

import (
	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20/rekey"
)

type SecureSession interface {
	Establish() (connection.Transport, connection.Crypto, *rekey.Controller, error)
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

func (c *DefaultSecureSession) Establish() (connection.Transport, connection.Crypto, *rekey.Controller, error) {
	crypto, controller, err := c.secret.Exchange(c.transport)
	if err != nil {
		return nil, nil, nil, err
	}

	return c.transport, crypto, controller, nil
}
