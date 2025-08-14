package network

import (
	"tungo/application"
)

type SecureSession interface {
	Establish() (application.ConnectionAdapter, application.CryptographyService, error)
}

type DefaultSecureSession struct {
	adapter application.ConnectionAdapter
	secret  Secret
}

func NewDefaultSecureSession(adapter application.ConnectionAdapter, secret Secret) *DefaultSecureSession {
	return &DefaultSecureSession{
		adapter: adapter,
		secret:  secret,
	}
}

func (c *DefaultSecureSession) Establish() (application.ConnectionAdapter, application.CryptographyService, error) {
	cryptographyService, cryptographyServiceErr := c.secret.Exchange(c.adapter)
	if cryptographyServiceErr != nil {
		return nil, nil, cryptographyServiceErr
	}

	return c.adapter, cryptographyService, nil
}
