package sysctl

type Contract interface {
	NetIpv4IpForward() ([]byte, error)
	WNetIpv4IpForward() ([]byte, error)
}
