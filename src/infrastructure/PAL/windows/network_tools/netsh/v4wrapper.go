//go:build windows

package netsh

import (
	"fmt"
	"strconv"
	"tungo/infrastructure/PAL"
)

// V4Wrapper is a IPv4 implementation of netsh.Contract
type V4Wrapper struct {
	commander PAL.Commander
}

func NewV4Wrapper(commander PAL.Commander) Contract {
	return &V4Wrapper{commander: commander}
}

func (w *V4Wrapper) DeleteDefaultRoute(ifName string) error {
	output, err := w.commander.CombinedOutput("netsh", "interface", "ipv4", "delete", "route", "0.0.0.0/0",
		"name="+`"`+ifName+`"`)
	if err != nil {
		return fmt.Errorf("DeleteDefaultRoute error: %v, output: %s", err, output)
	}
	return nil
}

func (w *V4Wrapper) DeleteAddress(ifName, interfaceAddress string) error {
	output, err := w.commander.CombinedOutput("netsh", "interface", "ip", "delete", "address",
		"name="+`"`+ifName+`"`, "addr="+interfaceAddress)
	if err != nil {
		return fmt.Errorf("DeleteAddress error: %v, output: %s", err, output)
	}
	return nil
}

func (w *V4Wrapper) SetDNS(ifName string, dnsServers []string) error {
	if len(dnsServers) == 0 {
		output, err := w.commander.CombinedOutput(
			"netsh", "interface", "ip", "set", "dns", "name="+`"`+ifName+`"`, "source=dhcp",
		)
		if err != nil {
			return fmt.Errorf("DNS set DHCP error: %v, output: %s", err, output)
		}
		return nil
	}
	// Otherwise: reset to DHCP first (best-effort)
	_, _ = w.commander.CombinedOutput(
		"netsh", "interface", "ip", "set", "dns", "name="+`"`+ifName+`"`, "source=dhcp",
	)

	// Manually set DNS servers
	for i, dns := range dnsServers {
		var args []string
		if i == 0 {
			args = []string{"interface", "ip", "set", "dns", "name=" + `"` + ifName + `"`, "static", dns, "primary"}
		} else {
			args = []string{"interface", "ip", "add", "dns", "name=" + `"` + ifName + `"`, dns, "index=" + strconv.Itoa(i+1)}
		}
		if output, err := w.commander.CombinedOutput("netsh", args...); err != nil {
			return fmt.Errorf("DNS setup error: %v, output: %s", err, output)
		}
	}
	return nil
}

func (w *V4Wrapper) SetMTU(ifName string, mtu int) error {
	output, err := w.commander.CombinedOutput(
		"netsh", "interface", "ipv4", "set", "subinterface",
		`"`+ifName+`"`, "mtu="+strconv.Itoa(mtu), "store=active",
	)
	if err != nil {
		return fmt.Errorf("SetInterfaceMTU error: %v, output: %s", err, output)
	}
	return nil
}

func (w *V4Wrapper) AddRoutePrefix(destinationPrefix, ifName string, metric int) error {
	output, err := w.commander.CombinedOutput("netsh", "interface", "ipv4", "add", "route",
		destinationPrefix, "interface="+`"`+ifName+`"`, "metric="+strconv.Itoa(metric), "store=active")
	if err != nil {
		return fmt.Errorf("AddRoutePrefix(%s) error: %v, output: %s", destinationPrefix, err, output)
	}
	return nil
}

func (w *V4Wrapper) DeleteRoutePrefix(destinationPrefix, ifName string) error {
	output, err := w.commander.CombinedOutput("netsh", "interface", "ipv4", "delete", "route",
		destinationPrefix, "name="+`"`+ifName+`"`)
	if err != nil {
		return fmt.Errorf("DeleteRoutePrefix(%s) error: %v, output: %s", destinationPrefix, err, output)
	}
	return nil
}

func (w *V4Wrapper) SetAddressStatic(ifName, ip, mask string) error {
	output, err := w.commander.CombinedOutput(
		"netsh", "interface", "ip", "set", "address",
		"name="+`"`+ifName+`"`, "static", ip, mask, "none",
	)
	if err != nil {
		return fmt.Errorf("SetAddressStatic error: %v, output: %s", err, output)
	}
	return nil
}

func (w *V4Wrapper) SetAddressWithGateway(ifName, ip, mask, gw string, metric int) error {
	output, err := w.commander.CombinedOutput(
		"netsh", "interface", "ip", "set", "address",
		"name="+`"`+ifName+`"`, "static", ip, mask, gw, strconv.Itoa(metric),
	)
	if err != nil {
		return fmt.Errorf("SetAddressWithGateway error: %v, output: %s", err, output)
	}
	return nil
}
