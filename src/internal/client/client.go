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

	"tungo/internal/client/resume"
	"tungo/internal/client/tcp"
	"tungo/internal/client/udp"
	clientconfig "tungo/internal/config/client"
	"tungo/internal/config/settings"
	"tungo/internal/trafficstats"
	clienttun "tungo/internal/tun/client"
)

var (
	ErrResumeDetected = errors.New("resume detected")
)

const (
	reconnectInterval = 500 * time.Millisecond
)

type tunManager interface {
	OpenTunnel(serverAddr netip.Addr) (io.ReadWriter, error)
	CloseTunnel() error
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

func New(configuration *clientconfig.Configuration) (*Client, error) {
	slog.Info("starting client")
	if configuration == nil {
		return nil, fmt.Errorf("client configuration is nil")
	}
	tunManager, err := clienttun.New(configuration)
	if err != nil {
		return nil, fmt.Errorf("init error: failed to configure tun: %w", err)
	}
	return &Client{
		configuration: configuration,
		tunManager:    tunManager,
	}, nil
}

func (c *Client) Ready() bool {
	return c.ready.Load()
}

// Run reconnects until the client is stopped.
// Context cancellation is a clean stop.
func (c *Client) Run(ctx context.Context) error {
	for ctx.Err() == nil {
		err := c.runSession(ctx)
		if err == nil || errors.Is(err, context.Canceled) {
			return nil
		}
		slog.Warn("session error, reconnecting", "err", err)
		timer := time.NewTimer(reconnectInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
	return nil
}

func (c *Client) runSession(parentCtx context.Context) error {
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()
	done := make(chan error)
	go func() {
		done <- c.runTunnel(ctx)
	}()
	select {
	case <-ctx.Done():
		cancel()
		<-done
		return ctx.Err()
	case <-resume.Watch(ctx):
		cancel()
		<-done
		return ErrResumeDetected
	case err := <-done:
		return err
	}
}

func (c *Client) runTunnel(
	ctx context.Context,
) error {
	c.closeTun(false) // clean stale
	transport, crypto, rekey, err := c.connect(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = transport.Close() }()
	serverAddr, err := transportServerAddr(transport)
	if err != nil {
		return err
	}
	tun, err := c.tunManager.OpenTunnel(serverAddr)
	if err != nil {
		slog.Error("failed to open tunnel", "err", err)
		return err
	}
	defer c.closeTun(true)
	tun = trafficstats.WrapTun(tun)
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

func transportServerAddr(transport io.ReadWriteCloser) (netip.Addr, error) {
	remoteProvider, ok := transport.(interface{ RemoteAddrPort() netip.AddrPort })
	if !ok {
		return netip.Addr{}, fmt.Errorf("transport %T does not expose its remote address", transport)
	}
	addrPort := remoteProvider.RemoteAddrPort()
	if !addrPort.IsValid() {
		return netip.Addr{}, fmt.Errorf("transport %T returned an invalid remote address", transport)
	}
	return addrPort.Addr().Unmap(), nil
}

func (c *Client) closeTun(logFail bool) {
	if err := c.tunManager.CloseTunnel(); err != nil && logFail {
		slog.Warn("failed to close tunnel", "err", err)
	}
}
