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

type failover interface {
	OnFail() (bool, string, error)
	Reset()
}

type tunManager interface {
	Open(serverAddr netip.Addr) (io.ReadWriter, error)
	Close() error
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
	configurations *clientconfig.Configurations
	tunManager     tunManager
	ready          atomic.Bool
	failover       failover
	sessionReadyAt time.Time
}

// New creates a client using the system configuration files.
// Each session loads the active configuration and initializes its own TUN.
func New(configurations *clientconfig.Configurations) (*Client, error) {
	slog.Info("starting client")
	return &Client{
		configurations: configurations,
		failover:       clientconfig.NewFailover(configurations, 3),
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
		c.attemptFailover(err)
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

func (c *Client) attemptFailover(err error) {
	// if session lived long enough - reset failover count
	if !c.sessionReadyAt.IsZero() && time.Since(c.sessionReadyAt) >= 5*time.Minute {
		c.failover.Reset()
	}
	if errors.Is(err, ErrResumeDetected) {
		return
	}
	if rotated, name, err := c.failover.OnFail(); err != nil {
		slog.Warn("failover failed", "configuration", name, "err", err)
	} else if rotated {
		slog.Info("failover activated", "configuration", name)
	}
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
		<-done //nolint:errcheck
		return ctx.Err()
	case <-resume.Watch(ctx):
		cancel()
		<-done //nolint:errcheck
		return ErrResumeDetected
	case err := <-done:
		return err
	}
}

func (c *Client) runTunnel(
	ctx context.Context,
) error {
	c.sessionReadyAt = time.Time{}
	if c.tunManager != nil {
		if err := c.tunManager.Close(); err != nil {
			slog.Warn("failed to clean stale tunnel state", "err", err)
		} else {
			c.tunManager = nil
		}
	}
	configuration, err := c.configurations.Active()
	if err != nil {
		return err
	}
	activeSettings, err := configuration.ActiveSettings()
	if err != nil {
		return err
	}
	transport, crypto, rekey, err := c.connect(ctx, *configuration)
	if err != nil {
		return err
	}
	defer func() { _ = transport.Close() }()
	serverAddr, err := transportServerAddr(transport)
	if err != nil {
		return err
	}
	c.tunManager, err = clienttun.New(configuration)
	if err != nil {
		return err
	}
	defer func() {
		if err := c.tunManager.Close(); err != nil {
			slog.Warn("failed to clean stale tunnel state", "err", err)
		} else {
			c.tunManager = nil
		}
	}()
	tun, err := c.tunManager.Open(serverAddr)
	if err != nil {
		return err
	}
	tun = trafficstats.WrapTun(tun)
	allowed := allowedSources(activeSettings)
	switch activeSettings.Protocol {
	case settings.UDP:
		tunnel, err := udp.New(ctx, transport, tun, crypto, rekey, allowed)
		if err != nil {
			return err
		}
		c.ready.Store(true)
		c.sessionReadyAt = time.Now()
		slog.Info("tunneling traffic via TUN device")
		return tunnel.Run()
	case settings.TCP, settings.WS, settings.WSS:
		tunnel := tcp.New(ctx, transport, tun, crypto, rekey, allowed)
		c.ready.Store(true)
		c.sessionReadyAt = time.Now()
		slog.Info("tunneling traffic via TUN device")
		return tunnel.Run()
	default:
		return fmt.Errorf("unsupported protocol %q", activeSettings.Protocol)
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
