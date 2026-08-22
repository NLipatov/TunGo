package sysctl

import (
	"tungo/internal/platform/command"
)

type Forwarding struct {
	commander command.Runner
}

func New(commander command.Runner) *Forwarding {
	return &Forwarding{commander: commander}
}

func (w *Forwarding) NetIpv4IpForward() ([]byte, error) {
	return w.commander.CombinedOutput("sysctl", "net.ipv4.ip_forward")
}
func (w *Forwarding) WNetIpv4IpForward() ([]byte, error) {
	return w.commander.CombinedOutput("sysctl", "-w", "net.ipv4.ip_forward=1")
}

func (w *Forwarding) NetIpv6ConfAllForwarding() ([]byte, error) {
	return w.commander.CombinedOutput("sysctl", "net.ipv6.conf.all.forwarding")
}

func (w *Forwarding) WNetIpv6ConfAllForwarding() ([]byte, error) {
	return w.commander.CombinedOutput("sysctl", "-w", "net.ipv6.conf.all.forwarding=1")
}
