package network

import (
	"context"
	"tungo/application/network/connection"
)

// SecureSessionWithDeadline is a decorator for SecureSession which allows cancellation via ctx
type SecureSessionWithDeadline struct {
	ctx              context.Context
	secureConnection SecureSession
}

func NewSecureSessionWithDeadline(ctx context.Context, secureConnection SecureSession) *SecureSessionWithDeadline {
	return &SecureSessionWithDeadline{
		ctx:              ctx,
		secureConnection: secureConnection,
	}
}

func (c *SecureSessionWithDeadline) Establish() (connection.Transport, connection.Crypto, error) {
	type result struct {
		transport connection.Transport
		crypto    connection.Crypto
		err       error
	}

	resultChan := make(chan result, 1)

	go func() {
		transport, crypto, err := c.secureConnection.Establish()
		resultChan <- result{transport: transport, crypto: crypto, err: err}
	}()

	select {
	case <-c.ctx.Done():
		return nil, nil, c.ctx.Err()
	case res := <-resultChan:
		return res.transport, res.crypto, res.err
	}
}
