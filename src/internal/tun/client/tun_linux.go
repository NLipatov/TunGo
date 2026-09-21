package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"strings"

	"tungo/internal/config/client"
	"tungo/internal/config/settings"
	"tungo/internal/platform/command"
	"tungo/internal/tun/internal/linux/defaultroute"
	"tungo/internal/tun/internal/linux/dns"
	"tungo/internal/tun/internal/linux/epoll"
	"tungo/internal/tun/internal/linux/ioctl"
	"tungo/internal/tun/internal/linux/ip"
	"tungo/internal/tun/internal/linux/mssclamp"
)

type dnsConfigurator interface {
	Set(ifName string, ipv4Resolvers, ipv6Resolvers []string) error
	Revert() error
}

type TUN struct {
	configuration             *client.Configuration
	settings                  settings.Settings
	dns                       dnsConfigurator
	ip                        ip.Contract
	ioctl                     ioctl.Contract
	mss                       mssclamp.Contract
	pinnedServerAddr          netip.Addr
	tun                       io.ReadWriteCloser
	defaultRouteWatcherCancel context.CancelFunc
	splitsv4                  []string
	splitsv6                  []string
}

// New creates a TUN from a normalized, validated client configuration.
func New(conf *client.Configuration) (*TUN, error) {
	active, err := conf.ActiveSettings()
	if err != nil {
		return nil, err
	}
	cmd := command.New()
	return &TUN{
		configuration: conf,
		settings:      active,
		dns:           dns.New(cmd),
		ip:            ip.New(cmd),
		ioctl:         ioctl.New(ioctl.NewLinuxIoctlCommander(), "/dev/net/tun"),
		mss:           mssclamp.NewManager(cmd),
		splitsv4:      conf.TunnelRoutesV4,
		splitsv6:      conf.TunnelRoutesV6,
	}, nil
}

func (t *TUN) Open(serverAddr netip.Addr) (io.ReadWriter, error) {
	if !serverAddr.IsValid() {
		return nil, fmt.Errorf("invalid server address %q", serverAddr)
	}
	serverAddr = serverAddr.Unmap()
	tunFile, openTunErr := t.ioctl.CreateTunInterface(t.settings.TunName)
	if openTunErr != nil {
		openErr := fmt.Errorf("failed to open TUN interface: %w", openTunErr)
		return nil, errors.Join(openErr, t.Close())
	}
	tun, err := epoll.New(tunFile)
	if err != nil {
		openErr := fmt.Errorf("failed to initialize TUN I/O: %w", err)
		return nil, errors.Join(openErr, tunFile.Close(), t.Close())
	}
	if err := t.watchDefaultRoute(tun); err != nil {
		slog.Warn("failed to configure default route watcher", "err", err)
	}
	t.tun = tun
	if err := t.configureTunnel(serverAddr); err != nil {
		openErr := fmt.Errorf("failed to configure client: %w", err)
		return nil, errors.Join(openErr, t.Close())
	}
	if err := t.setDNS(); err != nil {
		slog.Warn("failed to configure DNS", "interface", t.settings.TunName, "err", err)
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

func (t *TUN) configureTunnel(serverAddr netip.Addr) error {
	err := t.ip.LinkSetDevUp(t.settings.TunName)
	if err != nil {
		return err
	}
	if t.settings.HasIPv4() {
		cidr4, cidr4Err := t.settings.IPv4CIDR()
		if cidr4Err != nil {
			return cidr4Err
		}
		if err := t.ip.AddrAddDev(t.settings.TunName, cidr4); err != nil {
			return err
		}
	}

	if t.settings.HasIPv6() {
		cidr6, cidr6Err := t.settings.IPv6CIDR()
		if cidr6Err != nil {
			return cidr6Err
		}
		if err := t.ip.AddrAddDev(t.settings.TunName, cidr6); err != nil {
			return err
		}
	}

	if serverAddr.Is4() && t.settings.HasIPv4() || serverAddr.Is6() && t.settings.HasIPv6() {
		routeInfo, err := t.ip.RouteGet(serverAddr)
		if err != nil {
			return err
		}
		var viaGateway, devInterface string
		fields := strings.Fields(routeInfo)
		for i, field := range fields {
			if field == "via" && i+1 < len(fields) {
				viaGateway = fields[i+1]
			}
			if field == "dev" && i+1 < len(fields) {
				devInterface = fields[i+1]
			}
		}
		if devInterface == "" {
			return fmt.Errorf("failed to parse route to server IP")
		}
		if viaGateway == "" {
			err = t.ip.RouteReplaceDev(serverAddr, devInterface)
		} else {
			gateway, parseErr := netip.ParseAddr(viaGateway)
			if parseErr != nil {
				return fmt.Errorf("failed to parse route gateway %q: %w", viaGateway, parseErr)
			}
			err = t.ip.RouteReplaceViaDev(serverAddr, devInterface, gateway)
		}
		if err != nil {
			return fmt.Errorf("failed to replace route to server IP: %v", err)
		}
		t.pinnedServerAddr = serverAddr
	}

	if err := t.addSplitRoutes(serverAddr); err != nil {
		return err
	}

	if setMtuErr := t.ip.LinkSetDevMTU(t.settings.TunName, t.settings.MTU); setMtuErr != nil {
		return fmt.Errorf(
			"failed to set %d MTU for %s: %s", t.settings.MTU, t.settings.TunName, setMtuErr,
		)
	}

	families := mssclamp.Families{
		IPv4: t.settings.HasIPv4(),
		IPv6: t.settings.HasIPv6(),
	}
	if err := t.mss.Install(t.settings.TunName, families); err != nil {
		return fmt.Errorf("failed to install MSS clamping for %s: %v", t.settings.TunName, err)
	}

	return nil
}

func (t *TUN) addSplitRoutes(serverAddr netip.Addr) error {
	serverRoute := netip.PrefixFrom(serverAddr, serverAddr.BitLen()).String()
	if t.settings.HasIPv4() {
		splits := withoutRoutes(t.splitsv4, t.settings.IPv4Subnet.Masked().String(), serverRoute)
		if err := t.ip.RouteAddSplitDev(t.settings.TunName, splits); err != nil {
			return err
		}
	}
	if t.settings.HasIPv6() {
		splits := withoutRoutes(t.splitsv6, t.settings.IPv6Subnet.Masked().String(), serverRoute)
		if err := t.ip.Route6AddSplitDev(t.settings.TunName, splits); err != nil {
			return err
		}
	}
	return nil
}

func (t *TUN) setDNS() error {
	var ipv4Resolvers, ipv6Resolvers []string
	if t.settings.HasIPv4() {
		ipv4Resolvers = t.settings.DNSv4
	}
	if t.settings.HasIPv6() {
		ipv6Resolvers = t.settings.DNSv6
	}

	if err := t.dns.Set(t.settings.TunName, ipv4Resolvers, ipv6Resolvers); err != nil {
		return fmt.Errorf("set DNS on %s: %w", t.settings.TunName, err)
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

	cleanupErrs = append(cleanupErrs,
		t.removeTunInterface(t.configuration.TCPSettings),
		t.removeTunInterface(t.configuration.UDPSettings),
		t.removeTunInterface(t.configuration.WSSettings),
	)
	if t.pinnedServerAddr.IsValid() {
		if err := t.ip.RouteDel(t.pinnedServerAddr); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("delete route to server %s: %w", t.pinnedServerAddr, err))
		} else {
			t.pinnedServerAddr = netip.Addr{}
		}
	}
	return errors.Join(cleanupErrs...)
}

func (t *TUN) removeTunInterface(s settings.Settings) error {
	if s.TunName == "" {
		return nil
	}
	var errs []error
	if err := t.mss.Remove(s.TunName); err != nil {
		errs = append(errs, fmt.Errorf("remove MSS clamping for %s: %w", s.TunName, err))
	}
	if err := t.ip.LinkDelete(s.TunName); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
