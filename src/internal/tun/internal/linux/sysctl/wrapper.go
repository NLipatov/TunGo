package sysctl

import (
	"tungo/internal/platform/command"
)

type Wrapper struct {
	commander command.Runner
}

func NewWrapper(commander command.Runner) *Wrapper {
	return &Wrapper{commander: commander}
}

func (w *Wrapper) NetIpv4IpForward() ([]byte, error) {
	return w.commander.CombinedOutput("sysctl", "net.ipv4.ip_forward")
}
func (w *Wrapper) WNetIpv4IpForward() ([]byte, error) {
	return w.commander.CombinedOutput("sysctl", "-w", "net.ipv4.ip_forward=1")
}

func (w *Wrapper) NetIpv6ConfAllForwarding() ([]byte, error) {
	return w.commander.CombinedOutput("sysctl", "net.ipv6.conf.all.forwarding")
}

func (w *Wrapper) WNetIpv6ConfAllForwarding() ([]byte, error) {
	return w.commander.CombinedOutput("sysctl", "-w", "net.ipv6.conf.all.forwarding=1")
}
