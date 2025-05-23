package tcp_connection

import (
	"context"
	"net"
	"tungo/application"
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

func (c *SecureSessionWithDeadline) Establish() (net.Conn, application.CryptographyService, error) {
	type result struct {
		conn net.Conn
		sess application.CryptographyService
		err  error
	}

	resultChan := make(chan result, 1)

	go func() {
		conn, cryptographyService, err := c.secureConnection.Establish()
		resultChan <- result{conn: conn, sess: cryptographyService, err: err}
	}()

	select {
	case <-c.ctx.Done():
		return nil, nil, c.ctx.Err()
	case res := <-resultChan:
		return res.conn, res.sess, res.err
	}
}
