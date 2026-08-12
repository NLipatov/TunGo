package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"sync/atomic"
	"time"

	"tungo/internal/client/tcp"
	"tungo/internal/client/udp"
	"tungo/internal/config"
	clientconfig "tungo/internal/config/client"
	"tungo/internal/config/settings"
	"tungo/internal/trafficstats"
	clienttun "tungo/internal/tun/client"
)

type tunManager interface {
	CreateDevice() (io.ReadWriteCloser, error)
	DisposeDevices() error
	SetRouteEndpoint(netip.AddrPort)
}

type crypto interface {
	Encrypt([]byte) ([]byte, error)
	Decrypt([]byte) ([]byte, error)
}

type clientRekey interface {
	MaybeBuildRekeyInit(time.Time, []byte) ([]byte, bool, error)
	HandleRekeyAck(uint16, []byte) (bool, error)
	ObservePeerEpoch(uint16)
	ActivateSendEpoch(uint16)
}

// Client owns the client lifecycle: reconnects, transport sessions, TUN setup,
// packet forwarding, and cleanup.
type Client struct {
	configuration *clientconfig.Configuration
	tunManager    tunManager
	ready         atomic.Bool
}

// New builds a client that owns the transport and TUN lifecycles.
func New() (*Client, error) {
	control := config.NewClientControl()
	slog.Info("starting client")

	conf, err := control.Configuration()
	if err != nil {
		return nil, fmt.Errorf("init error: failed to read client configuration: %w", err)
	}
	tunManager, err := clienttun.New(conf)
	if err != nil {
		return nil, fmt.Errorf("init error: failed to configure tun: %w", err)
	}

	return &Client{
		configuration: conf,
		tunManager:    tunManager,
	}, nil
}

// Run reconnects until the client is stopped. Context cancellation is a clean
// stop.
func (c *Client) Run(ctx context.Context) error {
	err := c.run(ctx)
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func (c *Client) run(ctx context.Context) error {
	defer c.disposeDevices()

	for ctx.Err() == nil {
		err := c.runSession(ctx)
		switch {
		case err == nil:
			return nil
		case errors.Is(err, context.Canceled):
			return context.Canceled
		default:
			slog.Warn("session error, reconnecting", "err", err)
			timer := time.NewTimer(500 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return context.Canceled
			case <-timer.C:
			}
		}
	}
	return context.Canceled
}

func (c *Client) runSession(parentCtx context.Context) error {
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	c.disposeDevices()
	return c.forward(ctx)
}

func (c *Client) forward(ctx context.Context) error {
	c.tunManager.SetRouteEndpoint(netip.AddrPort{})

	transport, crypto, rekey, err := c.establishConnection(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = transport.Close() }()
	attachRouteEndpoint(transport, c.tunManager)

	device, err := c.tunManager.CreateDevice()
	if err != nil {
		slog.Error("failed to create TUN device", "err", err)
		return err
	}
	defer func() { _ = device.Close() }()

	return c.runTunnel(ctx, transport, trafficstats.WrapTun(device), crypto, rekey)
}

func (c *Client) Ready() bool {
	return c.ready.Load()
}

func (c *Client) disposeDevices() {
	if err := c.tunManager.DisposeDevices(); err != nil {
		slog.Warn("failed to dispose TUN devices", "err", err)
	}
}

func (c *Client) runTunnel(
	ctx context.Context,
	transport io.ReadWriteCloser,
	tun io.ReadWriteCloser,
	crypto crypto,
	rekey clientRekey,
) error {
	selected, err := c.configuration.ActiveSettings()
	if err != nil {
		return err
	}
	allowed := allowedSources(selected)
	switch selected.Protocol {
	case settings.UDP:
		tunnel, err := udp.New(ctx, transport, tun, crypto, rekey, allowed)
		if err != nil {
			return err
		}
		c.ready.Store(true)
		slog.Info("tunneling traffic via TUN device")
		return tunnel.Run()
	case settings.TCP, settings.WS, settings.WSS:
		tunnel := tcp.New(ctx, transport, tun, crypto, rekey, allowed)
		c.ready.Store(true)
		slog.Info("tunneling traffic via TUN device")
		return tunnel.Run()
	default:
		return fmt.Errorf("unsupported protocol %q", selected.Protocol)
	}
}

func allowedSources(s settings.Settings) map[netip.Addr]struct{} {
	allowed := make(map[netip.Addr]struct{}, 2)
	if s.IPv4.IsValid() {
		allowed[s.IPv4.Unmap()] = struct{}{}
	}
	if s.IPv6.IsValid() {
		allowed[s.IPv6.Unmap()] = struct{}{}
	}
	return allowed
}

func attachRouteEndpoint(transport io.ReadWriteCloser, tunManager tunManager) {
	remoteProvider, ok := transport.(interface{ RemoteAddrPort() netip.AddrPort })
	if !ok {
		return
	}
	addrPort := remoteProvider.RemoteAddrPort()
	if addrPort.IsValid() {
		tunManager.SetRouteEndpoint(addrPort)
	}
}
