package client

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/netip"

	appConfiguration "tungo/application/configuration"
	"tungo/application/configuration/settings"
	"tungo/infrastructure/telemetry/trafficstats"
	"tungo/infrastructure/tunnel/client/internal/tcp"
	"tungo/infrastructure/tunnel/client/internal/udp"
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

type rekeyController interface {
	ReadyForRekey() bool
	SendEpoch() uint16
	StartRekey(c2s, s2c []byte) (uint16, error)
	ActivateSendEpoch(uint16)
	ObservePeerEpoch(uint16)
	CurrentKeys() (clientToServer, serverToClient []byte)
}

type establishConnection func(context.Context) (
	io.ReadWriteCloser,
	crypto,
	rekeyController,
	error,
)

type runTunnel func(
	context.Context,
	io.ReadWriteCloser,
	io.ReadWriteCloser,
	crypto,
	rekeyController,
) (func() error, error)

// Client owns one complete client tunnel session: transport establishment,
// TUN setup, packet forwarding, and cleanup.
type Client struct {
	tunManager tunManager
	establish  establishConnection
	runTunnel  runTunnel
}

// New builds a client that owns transport and TUN session lifecycles.
func New(
	configuration appConfiguration.ClientRuntimeConfiguration,
	tunManager tunManager,
) *Client {
	connectionFactory := newConnection(configuration)
	return &Client{
		tunManager: tunManager,
		establish:  connectionFactory.EstablishConnection,
		runTunnel:  newTunnel(configuration),
	}
}

// Run establishes and runs one session. ready is called only after the
// transport and TUN device are ready to forward packets.
func (c *Client) Run(ctx context.Context, ready func()) error {
	c.tunManager.SetRouteEndpoint(netip.AddrPort{})

	transport, crypto, rekey, err := c.establish(ctx)
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

	run, err := c.runTunnel(ctx, transport, trafficstats.WrapTun(device), crypto, rekey)
	if err != nil {
		return err
	}
	ready()
	slog.Info("tunneling traffic via TUN device")
	return run()
}

func newTunnel(configuration appConfiguration.ClientRuntimeConfiguration) runTunnel {
	allowed := allowedSources(configuration.Settings)
	return func(
		ctx context.Context,
		transport io.ReadWriteCloser,
		tun io.ReadWriteCloser,
		crypto crypto,
		rekey rekeyController,
	) (func() error, error) {
		switch configuration.Settings.Protocol {
		case settings.UDP:
			client, err := udp.New(ctx, transport, tun, crypto, rekey, allowed)
			if err != nil {
				return nil, err
			}
			return client.Run, nil
		case settings.TCP, settings.WS, settings.WSS:
			return tcp.New(ctx, transport, tun, crypto, rekey, allowed).Run, nil
		default:
			return nil, fmt.Errorf("unsupported protocol %q", configuration.Settings.Protocol)
		}
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
