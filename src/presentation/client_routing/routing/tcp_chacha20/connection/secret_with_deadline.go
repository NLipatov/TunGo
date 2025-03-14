package connection

import (
	"context"
	"net"
	"tungo/infrastructure/cryptography/chacha20"
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

func (s SecretWithDeadline) Exchange(conn *net.Conn) (*chacha20.TcpCryptographyService, error) {
	type result struct {
		cryptographyService *chacha20.TcpCryptographyService
		err                 error
	}

	resultChan := make(chan result, 1)

	go func() {
		cryptographyService, err := s.secret.Exchange(conn)
		resultChan <- result{cryptographyService, err}
	}()

	select {
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	case res := <-resultChan:
		return res.cryptographyService, res.err
	}
}
