package udp

import (
	"context"
	"io"
	"net"
	"net/netip"
	"time"

	"tungo/internal/client/packetsender"
	udptransport "tungo/internal/transport/udp"
)

type sender interface {
	Send([]byte) error
}

type crypto interface {
	Encrypt([]byte) ([]byte, error)
	Decrypt([]byte) ([]byte, error)
}

type rekeySession interface {
	rekeyInitiator
	transportRekey
}

// Client moves packets between a TUN device and a UDP transport.
type Client struct {
	ctx       context.Context
	tun       *tunHandler
	transport *transportHandler
}

const (
	livenessCheckInterval = time.Second
	writeDeadline         = time.Second
)

// New builds the complete client UDP packet loop.
func New(
	ctx context.Context,
	conn *net.UDPConn,
	tun io.ReadWriter,
	crypto crypto,
	rekey rekeySession,
	allowedSources map[netip.Addr]struct{},
) *Client {
	udpTransport := udptransport.NewClientConn(conn, writeDeadline)
	outbound := packetsender.New(udpTransport, crypto)
	tunHandler := newTunHandler(ctx, tun, outbound, rekey, allowedSources)
	transportHandler := newTransportHandler(ctx, udpTransport, tun, crypto, rekey, outbound)

	return &Client{
		ctx:       ctx,
		tun:       tunHandler,
		transport: transportHandler,
	}
}

// Run moves packets in both directions until the context is cancelled or one
// direction fails.
func (c *Client) Run() error {
	errCh := make(chan error, 2)
	go func() { errCh <- c.tun.HandleTun() }()
	go func() { errCh <- c.transport.HandleTransport() }()

	t := time.NewTicker(livenessCheckInterval)
	defer t.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return c.ctx.Err()
		case err := <-errCh:
			return err
		case <-t.C:
			if err := c.transport.checkLiveness(); err != nil {
				return err
			}
		}
	}
}
