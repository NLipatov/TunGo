package tcp

import (
	"context"
	"io"
	"net/netip"
	"time"

	"tungo/application/configuration/settings"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/tunnel/internal/egress"
	"tungo/infrastructure/tunnel/internal/rekey"
)

type sender interface {
	Send([]byte) error
}

type crypto interface {
	Encrypt([]byte) ([]byte, error)
	Decrypt([]byte) ([]byte, error)
}

type rekeyController interface {
	ReadyForRekey() bool
	SendEpoch() uint16
	StartRekey(c2s, s2c []byte) (uint16, error)
	ActivateSendEpoch(uint16)
	ObservePeerEpoch(uint16)
	CurrentKeys() (clientToServer, serverToClient []byte)
}

// Client moves packets between a TUN device and a TCP-style transport.
type Client struct {
	ctx       context.Context
	tun       *tunHandler
	transport *transportHandler
}

// New builds the complete client packet loop used by TCP, WS, and WSS.
func New(
	ctx context.Context,
	transport io.ReadWriteCloser,
	tun io.ReadWriteCloser,
	crypto crypto,
	controller rekeyController,
	allowedSources map[netip.Addr]struct{},
) *Client {
	rekey := rekey.NewClientRekeyCoordinator(
		&primitives.DefaultKeyDeriver{},
		controller,
		settings.DefaultRekeyInterval,
		time.Now().UTC(),
	)
	outbound := egress.New(transport, crypto)
	tunHandler := newTunHandler(ctx, tun, outbound, rekey, allowedSources)
	transportHandler := newTransportHandler(ctx, transport, tun, crypto, controller, rekey, outbound)

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

	select {
	case <-c.ctx.Done():
		return c.ctx.Err()
	case err := <-errCh:
		return err
	}
}
