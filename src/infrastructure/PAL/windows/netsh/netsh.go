package netsh

import (
	"fmt"
	"os/exec"
	"strconv"
)

func RouteDelete(hostIP string) error {
	output, err := exec.Command("route", "delete", hostIP).CombinedOutput()
	if err != nil {
		return fmt.Errorf("RouteDelete error: %v, output: %s", err, output)
	}
	return nil
}

func InterfaceIPSetAddressStatic(interfaceName, hostIP, mask, gateway string) error {
	output, err := exec.Command("netsh", "interface", "ip", "set", "address",
		"name="+interfaceName, "static", hostIP, mask, gateway, "1").CombinedOutput()
	if err != nil {
		return fmt.Errorf("InterfaceIPSetAddressStatic error: %v, output: %s", err, output)
	}
	return nil
}

func InterfaceIPV4AddRouteDefault(interfaceName, gateway string) error {
	output, err := exec.Command("netsh", "interface", "ipv4", "add", "route", "0.0.0.0/0",
		"name="+interfaceName, gateway, "metric=1").CombinedOutput()
	if err != nil {
		return fmt.Errorf("InterfaceIPV4AddRouteDefault error: %v, output: %s", err, output)
	}
	return nil
}

func InterfaceIPV4DeleteAddress(IfName string) error {
	output, err := exec.Command("netsh", "interface", "ipv4", "delete", "route", "0.0.0.0/0",
		"name="+IfName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("InterfaceIPV4DeleteAddress error: %v, output: %s", err, output)
	}
	return nil
}

func InterfaceIPDeleteAddress(IfName, IfAddr string) error {
	output, err := exec.Command("netsh", "interface", "ip", "delete", "address",
		"name="+IfName, "addr="+IfAddr).CombinedOutput()
	if err != nil {
		return fmt.Errorf("InterfaceIPDeleteAddress error: %v, output: %s", err, output)
	}
	return nil
}

func SetInterfaceMetric(interfaceName string, metric int) error {
	output, err := exec.Command("netsh", "interface", "ipv4", "set", "interface",
		interfaceName, "metric="+strconv.Itoa(metric)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("SetInterfaceMetric error: %v, output: %s", err, output)
	}
	return nil
}
