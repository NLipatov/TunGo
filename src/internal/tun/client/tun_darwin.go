//go:build darwin

package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"slices"

	"tungo/internal/config/client"
	"tungo/internal/config/settings"
	"tungo/internal/platform/command"
	"tungo/internal/tun/internal/darwin/defaultroute"
	"tungo/internal/tun/internal/darwin/dns"
	"tungo/internal/tun/internal/darwin/ifconfig"
	"tungo/internal/tun/internal/darwin/route"
	"tungo/internal/tun/internal/darwin/utun"
)

type tun interface {
	io.ReadWriteCloser
	Name() string
}

type interfaceConfigurator interface {
	LinkAddrAdd(ifName string, prefix netip.Prefix) error
	SetMTU(ifName string, mtu int) error
}

type routeConfigurator interface {
	Add(destIP string) error
	AddSplit(ifName string, split []string) error
	Del(destIP string) error
}

type dnsConfigurator interface {
	Set(resolvers []string) error
	Revert() error
}

type TUN struct {
	settings                  settings.Settings
	tun                       tun
	dns                       dnsConfigurator
	ifconfig4                 interfaceConfigurator
	ifconfig6                 interfaceConfigurator
	route4                    routeConfigurator
	route6                    routeConfigurator
	pinnedServerAddr          netip.Addr
	defaultRouteWatcherCancel context.CancelFunc
	splitsv4                  []string
	splitsv6                  []string
}

// New creates a TUN from a normalized, validated client configuration.
func New(configuration *client.Configuration) (*TUN, error) {
	active, err := configuration.ActiveSettings()
	if err != nil {
		return nil, err
	}
	cmd := command.New()
	return &TUN{
		settings:  active,
		dns:       dns.New(cmd),
		ifconfig4: ifconfig.NewV4(cmd),
		ifconfig6: ifconfig.NewV6(cmd),
		route4:    route.NewV4(cmd),
		route6:    route.NewV6(cmd),
		splitsv4:  configuration.TunnelRoutesV4,
		splitsv6:  configuration.TunnelRoutesV6,
	}, nil
}

func (t *TUN) Open(serverAddr netip.Addr) (io.ReadWriter, error) {
	if !serverAddr.IsValid() {
		return nil, fmt.Errorf("invalid server address %q", serverAddr)
	}
	serverAddr = serverAddr.Unmap()
	tun, err := utun.New()
	if err != nil {
		return nil, fmt.Errorf("create utun: %w", err)
	}
	if err := t.watchDefaultRoute(tun); err != nil {
		slog.Warn("failed to configure default route watcher", "err", err)
	}
	t.tun = tun
	if err := t.setMTU(); err != nil {
		return nil, errors.Join(err, t.Close())
	}
	if err := t.pinServerRoute(serverAddr); err != nil {
		return nil, errors.Join(err, t.Close())
	}
	if err := t.assignAddresses(); err != nil {
		return nil, errors.Join(err, t.Close())
	}
	if err := t.addSplitRoutes(serverAddr); err != nil {
		return nil, errors.Join(err, t.Close())
	}
	if err := t.setDNS(); err != nil {
		slog.Warn("failed to configure DNS", "interface", t.tun.Name(), "err", err)
	}
	return t.tun, nil
}

func (t *TUN) watchDefaultRoute(tun io.Closer) error {
	ctx, cancel := context.WithCancel(context.Background())
	errCh, err := defaultroute.Watch(ctx)
	if err != nil {
		cancel()
		return err
	}
	go func() {
		select {
		case <-ctx.Done():
			return
		case err := <-errCh:
			if err != nil {
				slog.Warn("default route watcher failed", "err", err)
			} else {
				slog.Info("default route change detected")
			}
			_ = tun.Close()
		}
	}()
	t.defaultRouteWatcherCancel = cancel
	return nil
}

func (t *TUN) setDNS() error {
	resolvers := make([]string, 0, 4)
	if t.settings.HasIPv4() {
		resolvers = append(resolvers, t.settings.DNSv4...)
	}
	if t.settings.HasIPv6() {
		resolvers = append(resolvers, t.settings.DNSv6...)
	}

	if err := t.dns.Set(resolvers); err != nil {
		return fmt.Errorf("set DNS: %w", err)
	}
	return nil
}

func (t *TUN) setMTU() error {
	configurator := t.ifconfig4
	if !t.settings.HasIPv4() {
		configurator = t.ifconfig6
	}
	if err := configurator.SetMTU(t.tun.Name(), t.settings.MTU); err != nil {
		return fmt.Errorf("set MTU %d on %s: %w", t.settings.MTU, t.tun.Name(), err)
	}
	return nil
}

func (t *TUN) pinServerRoute(serverAddr netip.Addr) error {
	var route routeConfigurator
	switch {
	case serverAddr.Is4() && t.settings.HasIPv4():
		route = t.route4
	case serverAddr.Is6() && t.settings.HasIPv6():
		route = t.route6
	default:
		return nil
	}
	if err := route.Add(serverAddr.String()); err != nil {
		return fmt.Errorf("route to server %s: %w", serverAddr, err)
	}
	t.pinnedServerAddr = serverAddr
	return nil
}

func (t *TUN) assignAddresses() error {
	if t.settings.HasIPv4() {
		prefix := netip.PrefixFrom(t.settings.IPv4, t.settings.IPv4Subnet.Bits())
		if err := t.ifconfig4.LinkAddrAdd(t.tun.Name(), prefix); err != nil {
			return fmt.Errorf("set IPv4 address %s on %s: %w", prefix, t.tun.Name(), err)
		}
	}
	if t.settings.HasIPv6() {
		prefix := netip.PrefixFrom(t.settings.IPv6, t.settings.IPv6Subnet.Bits())
		if err := t.ifconfig6.LinkAddrAdd(t.tun.Name(), prefix); err != nil {
			return fmt.Errorf("set IPv6 address %s on %s: %w", prefix, t.tun.Name(), err)
		}
	}
	return nil
}

func (t *TUN) addSplitRoutes(serverAddr netip.Addr) error {
	serverRoute := netip.PrefixFrom(serverAddr, serverAddr.BitLen()).String()
	if t.settings.HasIPv4() {
		// Unlike Linux, macOS does not create an IPv4 subnet route when assigning
		// an address to utun, so add it explicitly.
		splits := slices.Clone(t.splitsv4)
		tunSubnet := t.settings.IPv4Subnet.Masked().String()
		if !slices.Contains(splits, tunSubnet) {
			splits = append(splits, tunSubnet)
		}
		splits = withoutRoutes(splits, serverRoute)
		if err := t.route4.AddSplit(t.tun.Name(), splits); err != nil {
			return fmt.Errorf("add IPv4 split default: %w", err)
		}
	}
	if t.settings.HasIPv6() {
		splits := withoutRoutes(t.splitsv6, t.settings.IPv6Subnet.Masked().String(), serverRoute)
		if err := t.route6.AddSplit(t.tun.Name(), splits); err != nil {
			return fmt.Errorf("add IPv6 split default: %w", err)
		}
	}
	return nil
}

func (t *TUN) Close() error {
	if t.defaultRouteWatcherCancel != nil {
		t.defaultRouteWatcherCancel()
	}
	var cleanupErrs []error
	if err := t.dns.Revert(); err != nil {
		cleanupErrs = append(cleanupErrs, fmt.Errorf("restore DNS: %w", err))
	}
	if t.tun != nil {
		if err := t.tun.Close(); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("close TUN: %w", err))
		}
		t.tun = nil
	}
	if t.pinnedServerAddr.IsValid() {
		var err error
		if t.pinnedServerAddr.Is4() {
			err = t.route4.Del(t.pinnedServerAddr.String())
		} else {
			err = t.route6.Del(t.pinnedServerAddr.String())
		}
		if err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("delete route to server %s: %w", t.pinnedServerAddr, err))
		} else {
			t.pinnedServerAddr = netip.Addr{}
		}
	}
	return errors.Join(cleanupErrs...)
}
