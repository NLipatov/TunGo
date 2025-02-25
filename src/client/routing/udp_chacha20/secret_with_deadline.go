package udp_chacha20

import (
	"context"
	"net"
	"tungo/crypto/chacha20"
)

type CancellableSecret struct {
	secret Secret
	ctx    context.Context
}

func NewCancellableSecret(ctx context.Context, secret Secret) CancellableSecret {
	return CancellableSecret{
		secret: secret,
		ctx:    ctx,
	}
}

func (s CancellableSecret) Exchange(conn *net.UDPConn) (*chacha20.UdpSession, error) {
	type result struct {
		session *chacha20.UdpSession
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
