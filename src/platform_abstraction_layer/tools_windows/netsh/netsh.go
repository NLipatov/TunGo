package netsh

import (
	"os/exec"
	"strconv"
)

func RouteDelete(hostIP string) error {
	return exec.Command("route", "delete", hostIP).Run()
}

func InterfaceIPSetAddressStatic(interfaceName, hostIP, mask, gateway string) error {
	return exec.
		Command("netsh", "interface", "ip", "set", "address",
			"name="+interfaceName, "static", hostIP, mask, gateway, "1").
		Run()
}

func InterfaceIPV4AddRouteDefault(interfaceName, gateway string) error {
	return exec.
		Command("netsh", "interface", "ipv4", "add", "route", "0.0.0.0/0",
			"name="+interfaceName, gateway, "metric=1").
		Run()
}

func InterfaceIPV4DeleteAddress(IfName string) error {
	return exec.
		Command("netsh", "interface", "ipv4", "delete", "route", "0.0.0.0/0",
			"name="+IfName).
		Run()
}

func InterfaceIPDeleteAddress(IfName, IfAddr string) error {
	return exec.
		Command("netsh", "interface", "ip", "delete", "address",
			"name="+IfName, "addr="+IfAddr).
		Run()
}

func SetInterfaceMetric(interfaceName string, metric int) error {
	return exec.
		Command("netsh", "interface", "ipv4", "set", "interface",
			interfaceName, "metric="+strconv.Itoa(metric)).
		Run()
}
