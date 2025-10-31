//go:build windows

package netsh

import (
	"fmt"
	"strconv"
	"tungo/infrastructure/PAL"
)

type Wrapper struct {
	commander PAL.Commander
}

func NewWrapper(commander PAL.Commander) Contract {
	return &Wrapper{commander: commander}
}

func (w *Wrapper) RouteDelete(hostIP string) error {
	output, err := w.commander.CombinedOutput("route", "delete", hostIP)
	if err != nil {
		return fmt.Errorf("RouteDelete error: %v, output: %s", err, output)
	}
	return nil
}

func (w *Wrapper) InterfaceDeleteDefaultRoute(interfaceName string) error {
	output, err := w.commander.CombinedOutput("netsh", "interface", "ipv4", "delete", "route", "0.0.0.0/0",
		"name="+`"`+interfaceName+`"`)
	if err != nil {
		return fmt.Errorf("InterfaceDeleteDefaultRoute error: %v, output: %s", err, output)
	}
	return nil
}

func (w *Wrapper) InterfaceIPDeleteAddress(interfaceName, interfaceAddress string) error {
	output, err := w.commander.CombinedOutput("netsh", "interface", "ip", "delete", "address",
		"name="+`"`+interfaceName+`"`, "addr="+interfaceAddress)
	if err != nil {
		return fmt.Errorf("InterfaceIPDeleteAddress error: %v, output: %s", err, output)
	}
	return nil
}

func (w *Wrapper) SetInterfaceMetric(interfaceName string, metric int) error {
	output, err := w.commander.CombinedOutput("netsh", "interface", "ipv4", "set", "interface",
		"name="+`"`+interfaceName+`"`, "metric="+strconv.Itoa(metric))
	if err != nil {
		return fmt.Errorf("SetInterfaceMetric error: %v, output: %s", err, output)
	}
	return nil
}

func (w *Wrapper) InterfaceSetDNSServers(interfaceName string, dnsServers []string) error {
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

func (w *Wrapper) LinkSetDevMTU(interfaceName string, mtu int) error {
	output, err := w.commander.CombinedOutput(
		"netsh", "interface", "ipv4", "set", "subinterface",
		`"`+interfaceName+`"`, "mtu="+strconv.Itoa(mtu), "store=active",
	)
	if err != nil {
		return fmt.Errorf("SetInterfaceMTU error: %v, output: %s", err, output)
	}
	return nil
}

func (w *Wrapper) InterfaceAddRouteOnLink(prefix, interfaceName string, metric int) error {
	output, err := w.commander.CombinedOutput("netsh", "interface", "ipv4", "add", "route",
		prefix, "interface="+`"`+interfaceName+`"`, "metric="+strconv.Itoa(metric), "store=active")
	if err != nil {
		return fmt.Errorf("InterfaceAddRouteOnLink(%s) error: %v, output: %s", prefix, err, output)
	}
	return nil
}

func (w *Wrapper) InterfaceDeleteRoute(prefix, interfaceName string) error {
	output, err := w.commander.CombinedOutput("netsh", "interface", "ipv4", "delete", "route",
		prefix, "name="+`"`+interfaceName+`"`)
	if err != nil {
		return fmt.Errorf("InterfaceDeleteRoute(%s) error: %v, output: %s", prefix, err, output)
	}
	return nil
}

func (w *Wrapper) InterfaceSetAddressNoGateway(interfaceName, ip, mask string) error {
	output, err := w.commander.CombinedOutput(
		"netsh", "interface", "ip", "set", "address",
		"name="+`"`+interfaceName+`"`, "static", ip, mask, "none",
	)
	if err != nil {
		return fmt.Errorf("InterfaceSetAddressNoGateway error: %v, output: %s", err, output)
	}
	return nil
}

func (w *Wrapper) InterfaceSetAddressWithGateway(interfaceName, ip, mask, gw string, metric int) error {
	output, err := w.commander.CombinedOutput(
		"netsh", "interface", "ip", "set", "address",
		"name="+`"`+interfaceName+`"`, "static", ip, mask, gw, strconv.Itoa(metric),
	)
	if err != nil {
		return fmt.Errorf("InterfaceSetAddressWithGateway error: %v, output: %s", err, output)
	}
	return nil
}
