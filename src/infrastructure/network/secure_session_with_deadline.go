package network

import (
	"context"
	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20/rekey"
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

func (c *SecureSessionWithDeadline) Establish() (connection.Transport, connection.Crypto, *rekey.StateMachine, error) {
	type result struct {
		transport connection.Transport
		crypto    connection.Crypto
		ctrl      *rekey.StateMachine
		err       error
	}

	resultChan := make(chan result, 1)

	go func() {
		transport, crypto, ctrl, err := c.secureConnection.Establish()
		resultChan <- result{transport: transport, crypto: crypto, ctrl: ctrl, err: err}
	}()

	select {
	case <-c.ctx.Done():
		return nil, nil, nil, c.ctx.Err()
	case res := <-resultChan:
		return res.transport, res.crypto, res.ctrl, res.err
	}
}
