//go:build windows

package netsh

import (
	"fmt"
	"strconv"
	"tungo/infrastructure/PAL"
)

// v4Wrapper is a IPv4 implementation of netsh.Contract
type v4Wrapper struct {
	commander PAL.Commander
}

func newV4Wrapper(commander PAL.Commander) Contract {
	return &v4Wrapper{
		commander: commander,
	}
}

func (w *v4Wrapper) DeleteDefaultRoute(ifName string) error {
	output, err := w.commander.CombinedOutput("netsh", "interface", "ipv4", "delete", "route", "0.0.0.0/0",
		"interface="+`"`+ifName+`"`)
	if err != nil {
		return fmt.Errorf("DeleteDefaultRoute error: %v, output: %s", err, output)
	}
	return nil
}

func (w *v4Wrapper) DeleteAddress(ifName, interfaceAddress string) error {
	output, err := w.commander.CombinedOutput("netsh", "interface", "ip", "delete", "address",
		"interface="+`"`+ifName+`"`, "addr="+interfaceAddress)
	if err != nil {
		return fmt.Errorf("DeleteAddress error: %v, output: %s", err, output)
	}
	return nil
}

func (w *v4Wrapper) SetDNS(ifName string, dnsServers []string) error {
	if len(dnsServers) == 0 {
		output, err := w.commander.CombinedOutput(
			"netsh", "interface", "ip", "set", "dns", "interface="+`"`+ifName+`"`, "source=dhcp",
		)
		if err != nil {
			return fmt.Errorf("DNS set DHCP error: %v, output: %s", err, output)
		}
		return nil
	}
	// Otherwise: reset to DHCP first (best-effort)
	_, _ = w.commander.CombinedOutput(
		"netsh", "interface", "ip", "set", "dns", "interface="+`"`+ifName+`"`, "source=dhcp",
	)

	// Manually set DNS servers
	for i, dns := range dnsServers {
		var args []string
		if i == 0 {
			args = []string{"interface", "ip", "set", "dns", "interface=" + `"` + ifName + `"`, "static", dns, "primary"}
		} else {
			args = []string{"interface", "ip", "add", "dns", "interface=" + `"` + ifName + `"`, dns, "index=" + strconv.Itoa(i+1)}
		}
		if output, err := w.commander.CombinedOutput("netsh", args...); err != nil {
			return fmt.Errorf("DNS setup error: %v, output: %s", err, output)
		}
	}
	return nil
}

func (w *v4Wrapper) SetMTU(ifName string, mtu int) error {
	output, err := w.commander.CombinedOutput(
		"netsh", "interface", "ipv4", "set", "subinterface",
		`"`+ifName+`"`, "mtu="+strconv.Itoa(mtu), "store=active",
	)
	if err != nil {
		return fmt.Errorf("SetInterfaceMTU error: %v, output: %s", err, output)
	}
	return nil
}

func (w *v4Wrapper) AddRoutePrefix(destinationPrefix, ifName string, metric int) error {
	output, err := w.commander.CombinedOutput("netsh", "interface", "ipv4", "add", "route",
		destinationPrefix, "interface="+`"`+ifName+`"`, "metric="+strconv.Itoa(metric), "store=active")
	if err != nil {
		return fmt.Errorf("AddRoutePrefix(%s) error: %v, output: %s", destinationPrefix, err, output)
	}
	return nil
}

func (w *v4Wrapper) DeleteRoutePrefix(destinationPrefix, ifName string) error {
	output, err := w.commander.CombinedOutput("netsh", "interface", "ipv4", "delete", "route",
		destinationPrefix, "interface="+`"`+ifName+`"`)
	if err != nil {
		return fmt.Errorf("DeleteRoutePrefix(%s) error: %v, output: %s", destinationPrefix, err, output)
	}
	return nil
}

func (w *v4Wrapper) SetAddressStatic(ifName, ip, mask string) error {
	output, err := w.commander.CombinedOutput(
		"netsh", "interface", "ip", "set", "address",
		"interface="+`"`+ifName+`"`, "static", ip, mask, "none",
	)
	if err != nil {
		return fmt.Errorf("SetAddressStatic error: %v, output: %s", err, output)
	}
	return nil
}

func (w *v4Wrapper) SetAddressWithGateway(ifName, ip, mask, gw string, metric int) error {
	output, err := w.commander.CombinedOutput(
		"netsh", "interface", "ip", "set", "address",
		"interface="+`"`+ifName+`"`, "static", ip, mask, gw, strconv.Itoa(metric),
	)
	if err != nil {
		return fmt.Errorf("SetAddressWithGateway error: %v, output: %s", err, output)
	}
	return nil
}

func (w *v4Wrapper) AddHostRouteViaGateway(hostIP, ifName, gateway string, metric int) error {
	args := []string{
		"interface", "ipv4", "add", "route",
		hostIP + "/32",
		"interface=" + `"` + ifName + `"`,
		"nexthop=" + gateway,
		"metric=" + strconv.Itoa(metric),
		"store=active",
	}
	out, err := w.commander.CombinedOutput("netsh", args...)
	if err != nil {
		return fmt.Errorf("AddHostRouteViaGateway error: %v, output: %s", err, out)
	}
	return nil
}

func (w *v4Wrapper) AddDefaultSplitRoutes(ifName string, metric int) error {
	halves := []string{"0.0.0.0/1", "128.0.0.0/1"}
	for _, p := range halves {
		out, err := w.commander.CombinedOutput(
			"netsh", "interface", "ipv4", "add", "route",
			p, "interface="+`"`+ifName+`"`, "metric="+strconv.Itoa(metric), "store=active",
		)
		if err != nil {
			return fmt.Errorf("AddDefaultSplitRoutes(v4 %s) error: %v, output: %s", p, err, out)
		}
	}
	return nil
}

func (w *v4Wrapper) DeleteDefaultSplitRoutes(ifName string) error {
	halves := []string{"0.0.0.0/1", "128.0.0.0/1"}
	var last error
	for _, p := range halves {
		if out, err := w.commander.CombinedOutput(
			"netsh", "interface", "ipv4", "delete", "route",
			p, "interface="+`"`+ifName+`"`,
		); err != nil {
			last = fmt.Errorf("DeleteDefaultSplitRoutes(v4 %s) error: %v, output: %s", p, err, out)
		}
	}
	return last
}

func (w *v4Wrapper) AddHostRouteOnLink(hostIP, ifName string, metric int) error {
	args := []string{
		"interface", "ipv4", "add", "route",
		hostIP + "/32",
		"interface=" + `"` + ifName + `"`,
		"nexthop=0.0.0.0",
		"metric=" + strconv.Itoa(metric),
		"store=active",
	}
	out, err := w.commander.CombinedOutput("netsh", args...)
	if err != nil {
		return fmt.Errorf("AddHostRouteOnLink(v4) error: %v, output: %s", err, out)
	}
	return nil
}
