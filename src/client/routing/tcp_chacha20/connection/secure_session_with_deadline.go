package connection

import (
	"context"
	"net"
	"tungo/crypto/chacha20"
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

func (c *SecureSessionWithDeadline) Establish() (*net.Conn, *chacha20.TcpCryptographyService, error) {
	type result struct {
		conn *net.Conn
		sess *chacha20.TcpCryptographyService
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
