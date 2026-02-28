package sysctl

type Contract interface {
	NetIpv4IpForward() ([]byte, error)
	WNetIpv4IpForward() ([]byte, error)
	NetIpv6ConfAllForwarding() ([]byte, error)
	WNetIpv6ConfAllForwarding() ([]byte, error)
}
