package udp_connection

import (
	"context"
	"net"
	"tungo/application"
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

func (s SecretWithDeadline) Exchange(conn *net.UDPConn) (application.CryptographyService, error) {
	type result struct {
		session application.CryptographyService
		err     error
	}

	resultChan := make(chan result, 1)

	go func() {
		session, err := s.secret.Exchange(conn)
		resultChan <- result{session, err}
	}()

	select {
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	case res := <-resultChan:
		return res.session, res.err
	}
}
