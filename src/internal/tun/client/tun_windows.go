//go:build windows

package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"strconv"
	"strings"

	"tungo/internal/config/client"
	"tungo/internal/config/settings"
	"tungo/internal/tun/internal/windows/defaultroute"
	"tungo/internal/tun/internal/windows/ipcfg"
	"tungo/internal/tun/internal/windows/wtun"

	"golang.zx2c4.com/wintun"
)

const windowsTunnelType = "TunGo"

type networkConfigurator interface {
	FlushDNS() error
	SetAddressStatic(ifName string, prefix netip.Prefix) error
	SetDNS(ifName string, dnsServers []string) error
	SetMTU(ifName string, mtu int) error
	AddHostRouteViaGateway(hostIP netip.Addr, ifName string, gateway netip.Addr) error
	AddHostRouteOnLink(hostIP netip.Addr, ifName string) error
	AddSplitRoutes(ifName string, split []string) error
	DeleteRoute(destination netip.Addr) error
	DeleteRouteOnInterface(destination netip.Addr, ifName string) error
	BestRoute(dest netip.Addr) (netip.Addr, string, int, int, error)
}

type TUN struct {
	configuration             *client.Configuration
	settings                  settings.Settings
	tun                       io.ReadWriteCloser
	netConfig4                networkConfigurator
	netConfig6                networkConfigurator
	pinnedServerAddr          netip.Addr
	pinnedServerIf            string
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
	return &TUN{
		configuration: configuration,
		settings:      active,
		netConfig4:    ipcfg.NewV4(),
		netConfig6:    ipcfg.NewV6(),
		splitsv4:      configuration.TunnelRoutesV4,
		splitsv6:      configuration.TunnelRoutesV6,
	}, nil
}

func (t *TUN) Open(serverAddr netip.Addr) (io.ReadWriter, error) {
	if !serverAddr.IsValid() {
		return nil, fmt.Errorf("invalid server address %q", serverAddr)
	}
	serverAddr = serverAddr.Unmap()
	tun, err := t.createTun()
	if err != nil {
		return nil, err
	}
	if err := t.watchDefaultRoute(tun); err != nil {
		slog.Warn("failed to configure default route watcher", "err", err)
	}
	t.tun = tun
	if err := t.pinServerRoute(serverAddr); err != nil {
		return nil, errors.Join(err, t.closeActiveTunnel())
	}
	if err := t.assignAddresses(); err != nil {
		return nil, errors.Join(err, t.closeActiveTunnel())
	}
	if err := t.addSplitRoutes(serverAddr); err != nil {
		return nil, errors.Join(err, t.closeActiveTunnel())
	}
	if err := t.setMTU(); err != nil {
		return nil, errors.Join(err, t.closeActiveTunnel())
	}
	if err := t.setDNS(); err != nil {
		slog.Warn("failed to configure DNS", "interface", t.settings.TunName, "err", err)
	}
	return t.tun, nil
}

func (t *TUN) createTun() (io.ReadWriteCloser, error) {
	adapter, err := wintun.CreateAdapter(t.settings.TunName, windowsTunnelType, nil)
	if err != nil {
		return nil, fmt.Errorf("create adapter: %w", err)
	}
	tun, err := wtun.NewTUN(adapter)
	if err != nil {
		_ = adapter.Close()
		return nil, err
	}
	return tun, nil
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

func (t *TUN) pinServerRoute(serverAddr netip.Addr) error {
	netConfig := t.configuratorFor(serverAddr)
	if netConfig == nil {
		return nil
	}
	gateway, ifName, ifIndex, _, err := netConfig.BestRoute(serverAddr)
	if err != nil {
		return err
	}
	ifName, err = routeInterfaceName(ifName, ifIndex)
	if err != nil {
		return err
	}
	_ = netConfig.DeleteRoute(serverAddr)
	_ = netConfig.DeleteRouteOnInterface(serverAddr, ifName)
	if gateway.IsValid() {
		err = netConfig.AddHostRouteViaGateway(serverAddr, ifName, gateway)
	} else {
		err = netConfig.AddHostRouteOnLink(serverAddr, ifName)
	}
	if err != nil {
		return err
	}
	t.pinnedServerAddr = serverAddr
	t.pinnedServerIf = ifName
	return nil
}

func (t *TUN) configuratorFor(addr netip.Addr) networkConfigurator {
	switch {
	case addr.Is4() && t.settings.HasIPv4():
		return t.netConfig4
	case addr.Is6() && t.settings.HasIPv6():
		return t.netConfig6
	default:
		return nil
	}
}

func (t *TUN) assignAddresses() error {
	if t.settings.HasIPv4() {
		prefix := netip.PrefixFrom(t.settings.IPv4, t.settings.IPv4Subnet.Bits())
		if err := t.netConfig4.SetAddressStatic(t.settings.TunName, prefix); err != nil {
			return fmt.Errorf("set IPv4 address: %w", err)
		}
	}
	if t.settings.HasIPv6() {
		prefix := netip.PrefixFrom(t.settings.IPv6, t.settings.IPv6Subnet.Bits())
		if err := t.netConfig6.SetAddressStatic(t.settings.TunName, prefix); err != nil {
			return fmt.Errorf("set IPv6 address: %w", err)
		}
	}
	return nil
}

func (t *TUN) addSplitRoutes(serverAddr netip.Addr) error {
	serverRoute := netip.PrefixFrom(serverAddr, serverAddr.BitLen()).String()
	if t.settings.HasIPv4() {
		splits := withoutRoutes(t.splitsv4, t.settings.IPv4Subnet.Masked().String(), serverRoute)
		if err := t.netConfig4.AddSplitRoutes(t.settings.TunName, splits); err != nil {
			return fmt.Errorf("add IPv4 split routes: %w", err)
		}
	}
	if t.settings.HasIPv6() {
		splits := withoutRoutes(t.splitsv6, t.settings.IPv6Subnet.Masked().String(), serverRoute)
		if err := t.netConfig6.AddSplitRoutes(t.settings.TunName, splits); err != nil {
			return fmt.Errorf("add IPv6 split routes: %w", err)
		}
	}
	return nil
}

func (t *TUN) setMTU() error {
	if t.settings.HasIPv4() {
		if err := t.netConfig4.SetMTU(t.settings.TunName, t.settings.MTU); err != nil {
			return fmt.Errorf("set IPv4 MTU: %w", err)
		}
	}
	if t.settings.HasIPv6() {
		if err := t.netConfig6.SetMTU(t.settings.TunName, t.settings.MTU); err != nil {
			return fmt.Errorf("set IPv6 MTU: %w", err)
		}
	}
	return nil
}

func (t *TUN) setDNS() error {
	configured := false
	if t.settings.HasIPv4() {
		if err := t.netConfig4.SetDNS(t.settings.TunName, t.settings.DNSv4); err != nil {
			return fmt.Errorf("set IPv4 DNS: %w", err)
		}
		configured = true
	}
	if t.settings.HasIPv6() {
		if err := t.netConfig6.SetDNS(t.settings.TunName, t.settings.DNSv6); err != nil {
			setupErr := fmt.Errorf("set IPv6 DNS: %w", err)
			if configured {
				if cleanupErr := t.netConfig4.SetDNS(t.settings.TunName, nil); cleanupErr != nil {
					return errors.Join(
						setupErr,
						fmt.Errorf("clear IPv4 DNS: %w", cleanupErr),
					)
				}
				_ = t.flushDNS(t.settings)
			}
			return setupErr
		}
	}
	// Resolver cache flushing is best-effort; the configured DNS servers are
	// already active and a flush failure must not tear the tunnel down.
	_ = t.flushDNS(t.settings)
	return nil
}

func (t *TUN) Close() error {
	cleanupErrs := []error{t.closeActiveTunnel()}
	activeTunName := t.settings.TunName
	for _, stale := range []settings.Settings{
		t.configuration.TCPSettings,
		t.configuration.UDPSettings,
		t.configuration.WSSettings,
	} {
		if stale.TunName == "" || stale.TunName == activeTunName {
			continue
		}
		if !stale.IPv4Subnet.IsValid() && !stale.IPv6Subnet.IsValid() {
			continue
		}
		if err := errors.Join(t.cleanupSettings(stale)...); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("clean stale TUN %s: %w", stale.TunName, err))
		}
	}
	return errors.Join(cleanupErrs...)
}

func (t *TUN) closeActiveTunnel() error {
	if t.defaultRouteWatcherCancel != nil {
		t.defaultRouteWatcherCancel()
	}
	cleanupErrs := t.cleanupSettings(t.settings)
	if t.tun != nil {
		if err := t.tun.Close(); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("close TUN: %w", err))
		}
		t.tun = nil
	}
	if t.pinnedServerAddr.IsValid() {
		netConfig := t.netConfig6
		if t.pinnedServerAddr.Is4() {
			netConfig = t.netConfig4
		}
		if err := netConfig.DeleteRouteOnInterface(t.pinnedServerAddr, t.pinnedServerIf); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("delete route %s on %s: %w", t.pinnedServerAddr, t.pinnedServerIf, err))
		} else {
			t.pinnedServerAddr = netip.Addr{}
			t.pinnedServerIf = ""
		}
	}
	return errors.Join(cleanupErrs...)
}

func (t *TUN) cleanupSettings(active settings.Settings) []error {
	var cleanupErrs []error
	if active.IPv4Subnet.IsValid() {
		if err := t.netConfig4.SetDNS(active.TunName, nil); err != nil && !errors.Is(err, ipcfg.ErrInterfaceNotFound) {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("clear IPv4 DNS: %w", err))
		}
	}
	if active.IPv6Subnet.IsValid() {
		if err := t.netConfig6.SetDNS(active.TunName, nil); err != nil && !errors.Is(err, ipcfg.ErrInterfaceNotFound) {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("clear IPv6 DNS: %w", err))
		}
	}
	_ = t.flushDNS(active)
	return cleanupErrs
}

func (t *TUN) flushDNS(active settings.Settings) error {
	var flushErrs []error
	if active.IPv4Subnet.IsValid() {
		if err := t.netConfig4.FlushDNS(); err != nil {
			flushErrs = append(flushErrs, fmt.Errorf("flush IPv4 DNS cache: %w", err))
		}
	}
	if active.IPv6Subnet.IsValid() {
		if err := t.netConfig6.FlushDNS(); err != nil {
			flushErrs = append(flushErrs, fmt.Errorf("flush IPv6 DNS cache: %w", err))
		}
	}
	return errors.Join(flushErrs...)
}

func routeInterfaceName(ifName string, ifIndex int) (string, error) {
	trimmed := strings.TrimSpace(ifName)
	if trimmed != "" {
		return trimmed, nil
	}
	if ifIndex <= 0 {
		return "", fmt.Errorf("best route returned empty interface name and invalid index %d", ifIndex)
	}
	return strconv.Itoa(ifIndex), nil
}
