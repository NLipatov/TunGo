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

func (w *Wrapper) InterfaceIPSetAddressStatic(interfaceName, hostIP, mask, gateway string) error {
	output, err := w.commander.CombinedOutput("netsh", "interface", "ip", "set", "address",
		"name="+interfaceName, "static", hostIP, mask, gateway, "1")
	if err != nil {
		return fmt.Errorf("InterfaceIPSetAddressStatic error: %v, output: %s", err, output)
	}
	return nil
}

func (w *Wrapper) InterfaceIPV4AddRouteDefault(interfaceName, gateway string) error {
	output, err := w.commander.CombinedOutput("netsh", "interface", "ipv4", "add", "route", "0.0.0.0/0",
		"name="+interfaceName, gateway, "metric=1")
	if err != nil {
		return fmt.Errorf("InterfaceIPV4AddRouteDefault error: %v, output: %s", err, output)
	}
	return nil
}

func (w *Wrapper) InterfaceIPV4DeleteAddress(IfName string) error {
	output, err := w.commander.CombinedOutput("netsh", "interface", "ipv4", "delete", "route", "0.0.0.0/0",
		"name="+IfName)
	if err != nil {
		return fmt.Errorf("InterfaceIPV4DeleteAddress error: %v, output: %s", err, output)
	}
	return nil
}

func (w *Wrapper) InterfaceIPDeleteAddress(IfName, IfAddr string) error {
	output, err := w.commander.CombinedOutput("netsh", "interface", "ip", "delete", "address",
		"name="+IfName, "addr="+IfAddr)
	if err != nil {
		return fmt.Errorf("InterfaceIPDeleteAddress error: %v, output: %s", err, output)
	}
	return nil
}

func (w *Wrapper) SetInterfaceMetric(interfaceName string, metric int) error {
	output, err := w.commander.CombinedOutput("netsh", "interface", "ipv4", "set", "interface",
		interfaceName, "metric="+strconv.Itoa(metric))
	if err != nil {
		return fmt.Errorf("SetInterfaceMetric error: %v, output: %s", err, output)
	}
	return nil
}

func (w *Wrapper) InterfaceSetDNSServers(interfaceName string, dnsServers []string) error {
	// Cleanup current DNS settings
	_, _ = w.commander.CombinedOutput(
		"netsh", "interface", "ip", "set", "dns", "name="+interfaceName, "source=dhcp",
	)

	// Manually set DNS servers
	for i, dns := range dnsServers {
		var args []string
		if i == 0 {
			args = []string{"interface", "ip", "set", "dns", "name=" + interfaceName, "static", dns, "primary"}
		} else {
			args = []string{"interface", "ip", "add", "dns", "name=" + interfaceName, dns, "index=" + strconv.Itoa(i+1)}
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
		interfaceName, "mtu="+strconv.Itoa(mtu), "store=active",
	)
	if err != nil {
		return fmt.Errorf("SetInterfaceMTU error: %v, output: %s", err, output)
	}
	return nil
}
