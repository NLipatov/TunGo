package sysctl

import "os/exec"

func NetIpv4IpForward() ([]byte, error) {
	cmd := exec.Command("sysctl", "net.ipv4.ip_forward")
	return cmd.CombinedOutput()
}
func WNetIpv4IpForward() ([]byte, error) {
	cmd := exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1")
	return cmd.CombinedOutput()
}
