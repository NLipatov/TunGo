package ip

import (
	"fmt"
	"net/netip"
	"strings"

	"tungo/internal/tun/internal/splitroute"
)

type runner interface {
	CombinedOutput(name string, args ...string) ([]byte, error)
	Output(name string, args ...string) ([]byte, error)
}

// Configurator manages Linux network interfaces and routes through iproute2.
type Configurator struct {
	runner runner
}

func New(runner runner) *Configurator {
	return &Configurator{runner: runner}
}

// TunTapAddDevTun Adds new TUN device
func (i *Configurator) TunTapAddDevTun(devName string) error {
	createTunOutput, err := i.runner.CombinedOutput("ip", "tuntap",
		"add", "dev", devName, "mode", "tun")
	if err != nil {
		return fmt.Errorf("failed to create TUN %v: %v, output: %s", devName, err, createTunOutput)
	}

	return nil
}

// LinkDelete Deletes network device by name
func (i *Configurator) LinkDelete(devName string) error {
	output, err := i.runner.CombinedOutput("ip", "link", "delete", devName)
	if err != nil {
		return fmt.Errorf("failed to delete interface: %v, output: %s", err, output)
	}

	return nil
}

// LinkSetDevUp Sets network device status as UP
func (i *Configurator) LinkSetDevUp(devName string) error {
	startTunOutput, err := i.runner.CombinedOutput("ip", "link", "set", "dev", devName, "up")
	if err != nil {
		return fmt.Errorf("failed to start TUN %v: %v, output: %s", devName, err, startTunOutput)
	}

	return nil
}

// AddrAddDev Assigns an IP to a network device
func (i *Configurator) AddrAddDev(devName string, cidr string) error {
	output, assignIPErr := i.runner.CombinedOutput("ip", "addr", "add", cidr, "dev", devName)
	if assignIPErr != nil {
		return fmt.Errorf("failed to assign IP to TUN %v: %v, output: %s", devName, assignIPErr, output)
	}

	return nil
}

// RouteDefault gets the default network device name.
// It checks the IPv4 routing table first, then falls back to IPv6.
func (i *Configurator) RouteDefault() (string, error) {
	if iface, err := i.parseDefaultRoute("ip", "route"); err == nil {
		return iface, nil
	}
	if iface, err := i.parseDefaultRoute("ip", "-6", "route"); err == nil {
		return iface, nil
	}
	return "", fmt.Errorf("failed to get default interface from IPv4 or IPv6 routing table")
}

// parseDefaultRoute runs the given command and extracts the interface name
// from the first "default" route line by searching for the "dev" keyword.
func (i *Configurator) parseDefaultRoute(name string, args ...string) (string, error) {
	out, err := i.runner.Output(name, args...)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "default") {
			fields := strings.Fields(line)
			for j, f := range fields {
				if f == "dev" && j+1 < len(fields) {
					return fields[j+1], nil
				}
			}
		}
	}
	return "", fmt.Errorf("no default route found")
}

// RouteAddSplitDefaultDev adds IPv4 split default routes (0.0.0.0/1 + 128.0.0.0/1)
// through the given device. These are more specific than 0.0.0.0/0 so they take
// priority without replacing the original default route. When the TUN device is
// deleted, the kernel removes these routes automatically.
func (i *Configurator) RouteAddSplitDefaultDev(devName string) error {
	for _, prefix := range []string{splitroute.IPv4LowerHalf, splitroute.IPv4UpperHalf} {
		output, err := i.runner.CombinedOutput("ip", "route", "add", prefix, "dev", devName)
		if err != nil {
			return fmt.Errorf("failed to add split route %s via %s: %v, output: %s",
				prefix, devName, err, output)
		}
	}
	return nil
}

// Route6AddSplitDefaultDev adds IPv6 split default routes (::/1 + 8000::/1)
// through the given device.
func (i *Configurator) Route6AddSplitDefaultDev(devName string) error {
	for _, prefix := range []string{splitroute.IPv6LowerHalf, splitroute.IPv6UpperHalf} {
		output, err := i.runner.CombinedOutput("ip", "-6", "route", "add", prefix, "dev", devName)
		if err != nil {
			return fmt.Errorf("failed to add IPv6 split route %s via %s: %v, output: %s",
				prefix, devName, err, output)
		}
	}
	return nil
}

// RouteDelSplitDefault removes IPv4 split default routes through the given device.
func (i *Configurator) RouteDelSplitDefault(devName string) error {
	for _, prefix := range []string{splitroute.IPv4LowerHalf, splitroute.IPv4UpperHalf} {
		_, _ = i.runner.CombinedOutput("ip", "route", "del", prefix, "dev", devName)
	}
	return nil
}

// Route6DelSplitDefault removes IPv6 split default routes through the given device.
func (i *Configurator) Route6DelSplitDefault(devName string) error {
	for _, prefix := range []string{splitroute.IPv6LowerHalf, splitroute.IPv6UpperHalf} {
		_, _ = i.runner.CombinedOutput("ip", "-6", "route", "del", prefix, "dev", devName)
	}
	return nil
}

// RouteGet gets the route to a host.
func (i *Configurator) RouteGet(hostAddr netip.Addr) (string, error) {
	hostAddr = hostAddr.Unmap()
	family, err := familyArg(hostAddr)
	if err != nil {
		return "", err
	}
	routeBytes, err := i.runner.Output("ip", family, "route", "get", hostAddr.String())
	if err != nil {
		return "", fmt.Errorf("failed to get route to server IP: %v", err)
	}

	return string(routeBytes), nil
}

// RouteReplaceDev ensures that a host route exists via the device.
func (i *Configurator) RouteReplaceDev(hostAddr netip.Addr, ifName string) error {
	hostAddr = hostAddr.Unmap()
	family, err := familyArg(hostAddr)
	if err != nil {
		return err
	}
	output, err := i.runner.CombinedOutput(
		"ip", family, "route", "replace", hostAddr.String(), "dev", ifName,
	)
	if err != nil {
		return fmt.Errorf("failed to replace route: %s, output: %s", err, output)
	}
	return nil
}

// RouteReplaceViaDev ensures that a host route exists via the gateway and device.
func (i *Configurator) RouteReplaceViaDev(hostAddr netip.Addr, ifName string, gateway netip.Addr) error {
	hostAddr = hostAddr.Unmap()
	family, err := familyArg(hostAddr)
	if err != nil {
		return err
	}
	if !gateway.IsValid() {
		return fmt.Errorf("invalid route gateway %q", gateway)
	}
	gateway = gateway.Unmap()
	output, err := i.runner.CombinedOutput(
		"ip", family, "route", "replace", hostAddr.String(), "via", gateway.String(), "dev", ifName,
	)
	if err != nil {
		return fmt.Errorf("failed to replace route: %s, output: %s", err, output)
	}
	return nil
}

// RouteDel deletes a route to a host.
func (i *Configurator) RouteDel(hostAddr netip.Addr) error {
	hostAddr = hostAddr.Unmap()
	family, err := familyArg(hostAddr)
	if err != nil {
		return err
	}
	output, err := i.runner.CombinedOutput("ip", family, "route", "del", hostAddr.String())
	if err != nil {
		return fmt.Errorf("failed to del route: %s, output: %s", err, output)
	}
	return nil
}

func familyArg(addr netip.Addr) (string, error) {
	switch {
	case addr.Is4():
		return "-4", nil
	case addr.Is6():
		return "-6", nil
	default:
		return "", fmt.Errorf("invalid route address %q", addr)
	}
}

// LinkSetDevMTU sets device MTU
func (i *Configurator) LinkSetDevMTU(devName string, mtu int) error {
	output, err := i.runner.CombinedOutput("ip", "link",
		"set", "dev", devName, "mtu", fmt.Sprintf("%d", mtu))
	if err != nil {
		return fmt.Errorf("failed to set mtu: %s, output: %s", err, output)
	}
	return err
}
