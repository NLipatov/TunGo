package tun_tcp

import (
	"context"
	"fmt"
	"net"
	"time"
	"tungo/handshake/chacha20"
	"tungo/handshake/chacha20/chacha20_handshake"
	"tungo/settings"
)

type tcpConnectionBuilder struct {
	settings    settings.ConnectionSettings
	conn        net.Conn
	session     *chacha20.Session
	ctx         context.Context
	dialCancel  context.CancelFunc
	dialTimeout time.Duration
	err         error
}

func newTCPConnectionBuilder() *tcpConnectionBuilder {
	return &tcpConnectionBuilder{}
}

func (b *tcpConnectionBuilder) useSettings(s settings.ConnectionSettings) *tcpConnectionBuilder {
	if b.err != nil {
		return b
	}

	if s.ConnectionIP == "" || s.Port == "" {
		b.err = fmt.Errorf("invalid connection settings: IP and Port are required")
		return b
	}

	b.settings = s
	return b
}

func (b *tcpConnectionBuilder) useConnectionTimeout(duration time.Duration) *tcpConnectionBuilder {
	if b.err != nil {
		return b
	}

	b.dialTimeout = duration
	return b
}

func (b *tcpConnectionBuilder) connect(ctx context.Context) *tcpConnectionBuilder {
	if b.err != nil {
		return b
	}

	dialer := &net.Dialer{}
	if b.dialTimeout == 0 {
		b.dialTimeout = time.Second * 5
	}
	dialCtx, cancel := context.WithTimeout(ctx, b.dialTimeout)
	defer cancel()
	conn, err := dialer.DialContext(dialCtx, "tcp", fmt.Sprintf("%s%s", b.settings.ConnectionIP, b.settings.Port))
	if err != nil {
		if b.conn != nil {
			_ = b.conn.Close()
		}
		b.err = err
		return b
	}

	b.conn = conn
	b.ctx = ctx
	return b
}

func (b *tcpConnectionBuilder) handshake() *tcpConnectionBuilder {
	if b.err != nil {
		return b
	}

	session, err := chacha20_handshake.OnConnectedToServer(b.conn, b.settings)
	if err != nil {
		b.err = fmt.Errorf("aborting connection: registration failed: %s\n", err)
	}

	b.session = session
	return b
}

func (b *tcpConnectionBuilder) build() (net.Conn, *chacha20.Session, error) {
	if b.err != nil {
		return nil, nil, b.err
	}
	return b.conn, b.session, b.err
}
