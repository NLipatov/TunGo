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

func (w *V4Wrapper) IPDeleteDefaultRoute(interfaceName string) error {
	output, err := w.commander.CombinedOutput("netsh", "interface", "ipv4", "delete", "route", "0.0.0.0/0",
		"name="+`"`+interfaceName+`"`)
	if err != nil {
		return fmt.Errorf("IPDeleteDefaultRoute error: %v, output: %s", err, output)
	}
	return nil
}

func (w *V4Wrapper) IPDeleteAddress(interfaceName, interfaceAddress string) error {
	output, err := w.commander.CombinedOutput("netsh", "interface", "ip", "delete", "address",
		"name="+`"`+interfaceName+`"`, "addr="+interfaceAddress)
	if err != nil {
		return fmt.Errorf("IPDeleteAddress error: %v, output: %s", err, output)
	}
	return nil
}

func (w *V4Wrapper) IPSetDNS(interfaceName string, dnsServers []string) error {
	if len(dnsServers) == 0 {
		output, err := w.commander.CombinedOutput(
			"netsh", "interface", "ip", "set", "dns", "name="+`"`+interfaceName+`"`, "source=dhcp",
		)
		if err != nil {
			return fmt.Errorf("DNS set DHCP error: %v, output: %s", err, output)
		}
		return nil
	}
	// Otherwise: reset to DHCP first (best-effort)
	_, _ = w.commander.CombinedOutput(
		"netsh", "interface", "ip", "set", "dns", "name="+`"`+interfaceName+`"`, "source=dhcp",
	)

	// Manually set DNS servers
	for i, dns := range dnsServers {
		var args []string
		if i == 0 {
			args = []string{"interface", "ip", "set", "dns", "name=" + `"` + interfaceName + `"`, "static", dns, "primary"}
		} else {
			args = []string{"interface", "ip", "add", "dns", "name=" + `"` + interfaceName + `"`, dns, "index=" + strconv.Itoa(i+1)}
		}
		if output, err := w.commander.CombinedOutput("netsh", args...); err != nil {
			return fmt.Errorf("DNS setup error: %v, output: %s", err, output)
		}
	}
	return nil
}

func (w *V4Wrapper) IPSetMTU(interfaceName string, mtu int) error {
	output, err := w.commander.CombinedOutput(
		"netsh", "interface", "ipv4", "set", "subinterface",
		`"`+interfaceName+`"`, "mtu="+strconv.Itoa(mtu), "store=active",
	)
	if err != nil {
		return fmt.Errorf("SetInterfaceMTU error: %v, output: %s", err, output)
	}
	return nil
}

func (w *V4Wrapper) AddRoutePrefix(destinationPrefix, interfaceName string, metric int) error {
	output, err := w.commander.CombinedOutput("netsh", "interface", "ipv4", "add", "route",
		destinationPrefix, "interface="+`"`+interfaceName+`"`, "metric="+strconv.Itoa(metric), "store=active")
	if err != nil {
		return fmt.Errorf("AddRoutePrefix(%s) error: %v, output: %s", destinationPrefix, err, output)
	}
	return nil
}

func (w *V4Wrapper) IPDeleteRoutePrefix(destinationPrefix, interfaceName string) error {
	output, err := w.commander.CombinedOutput("netsh", "interface", "ipv4", "delete", "route",
		destinationPrefix, "name="+`"`+interfaceName+`"`)
	if err != nil {
		return fmt.Errorf("IPDeleteRoutePrefix(%s) error: %v, output: %s", destinationPrefix, err, output)
	}
	return nil
}

func (w *V4Wrapper) IPSetAddressStatic(interfaceName, ip, mask string) error {
	output, err := w.commander.CombinedOutput(
		"netsh", "interface", "ip", "set", "address",
		"name="+`"`+interfaceName+`"`, "static", ip, mask, "none",
	)
	if err != nil {
		return fmt.Errorf("IPSetAddressStatic error: %v, output: %s", err, output)
	}
	return nil
}

func (w *V4Wrapper) IPSetAddressWithGateway(interfaceName, ip, mask, gw string, metric int) error {
	output, err := w.commander.CombinedOutput(
		"netsh", "interface", "ip", "set", "address",
		"name="+`"`+interfaceName+`"`, "static", ip, mask, gw, strconv.Itoa(metric),
	)
	if err != nil {
		return fmt.Errorf("IPSetAddressWithGateway error: %v, output: %s", err, output)
	}
	return nil
}
