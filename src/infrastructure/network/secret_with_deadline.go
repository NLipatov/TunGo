package network

import (
	"context"
	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20/rekey"
)

// SecretWithDeadline is a decorator for Secret which allows cancellation via ctx
type SecretWithDeadline struct {
	secret Secret
	ctx    context.Context
}

func NewSecretWithDeadline(ctx context.Context, secret Secret) SecretWithDeadline {
	return SecretWithDeadline{
		secret: secret,
		ctx:    ctx,
	}
}

func (s SecretWithDeadline) Exchange(transport connection.Transport) (connection.Crypto, *rekey.StateMachine, error) {
	type result struct {
		cryptographyService connection.Crypto
		controller          *rekey.StateMachine
		err                 error
	}

	resultChan := make(chan result, 1)

	go func() {
		crypto, ctrl, cryptoErr := s.secret.Exchange(transport)
		resultChan <- result{crypto, ctrl, cryptoErr}
	}()

	select {
	case <-s.ctx.Done():
		return nil, nil, s.ctx.Err()
	case res := <-resultChan:
		return res.cryptographyService, res.controller, res.err
	}
}
