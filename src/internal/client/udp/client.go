package udp

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"time"

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

// New builds the complete client UDP packet loop.
func New(
	ctx context.Context,
	transport io.ReadWriteCloser,
	tun io.ReadWriter,
	crypto crypto,
	rekey rekeySession,
	allowedSources map[netip.Addr]struct{},
) (*Client, error) {
	udpConn, err := unwrapUDPConn(transport)
	if err != nil {
		return nil, err
	}

	const deadline = time.Second
	udpTransport := udptransport.NewClientConn(udpConn, deadline, deadline)
	outbound := newPacketSender(udpTransport, crypto)
	tunHandler := newTunHandler(ctx, tun, outbound, rekey, allowedSources)
	transportHandler := newTransportHandler(ctx, udpTransport, tun, crypto, rekey, outbound)

	return &Client{
		ctx:       ctx,
		tun:       tunHandler,
		transport: transportHandler,
	}, nil
}

// Run moves packets in both directions until the context is cancelled or one
// direction fails.
func (c *Client) Run() error {
	errCh := make(chan error, 2)
	go func() { errCh <- c.tun.HandleTun() }()
	go func() { errCh <- c.transport.HandleTransport() }()

	select {
	case <-c.ctx.Done():
		return c.ctx.Err()
	case err := <-errCh:
		return err
	}
}

func unwrapUDPConn(transport io.ReadWriteCloser) (*net.UDPConn, error) {
	current := transport
	for range 8 {
		if udpConn, ok := current.(*net.UDPConn); ok {
			return udpConn, nil
		}
		unwrapper, ok := current.(interface{ Unwrap() io.ReadWriteCloser })
		if !ok {
			break
		}
		next := unwrapper.Unwrap()
		if next == nil || next == current {
			break
		}
		current = next
	}
	return nil, fmt.Errorf("udp transport must wrap *net.UDPConn, got %T", transport)
}
