package sysctl

import (
	"tungo/infrastructure/PAL"
)

type Wrapper struct {
	commander PAL.Commander
}

func NewWrapper(commander PAL.Commander) *Wrapper {
	return &Wrapper{commander: commander}
}

func (w *Wrapper) NetIpv4IpForward() ([]byte, error) {
	return w.commander.CombinedOutput("sysctl", "net.ipv4.ip_forward")
}
func (w *Wrapper) WNetIpv4IpForward() ([]byte, error) {
	return w.commander.CombinedOutput("sysctl", "-w", "net.ipv4.ip_forward=1")
}
