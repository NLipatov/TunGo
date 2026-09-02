package sysctl

type runner interface {
	CombinedOutput(name string, args ...string) ([]byte, error)
}

type Forwarding struct {
	runner runner
}

func New(runner runner) *Forwarding {
	return &Forwarding{runner: runner}
}

func (w *Forwarding) NetIpv4IpForward() ([]byte, error) {
	return w.runner.CombinedOutput("sysctl", "net.ipv4.ip_forward")
}
func (w *Forwarding) WNetIpv4IpForward() ([]byte, error) {
	return w.runner.CombinedOutput("sysctl", "-w", "net.ipv4.ip_forward=1")
}

func (w *Forwarding) NetIpv6ConfAllForwarding() ([]byte, error) {
	return w.runner.CombinedOutput("sysctl", "net.ipv6.conf.all.forwarding")
}

func (w *Forwarding) WNetIpv6ConfAllForwarding() ([]byte, error) {
	return w.runner.CombinedOutput("sysctl", "-w", "net.ipv6.conf.all.forwarding=1")
}
