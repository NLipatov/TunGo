package ifconfig

type Contract interface {
	LinkAddrAdd(ifName, cidr string) error
	LinkAddrAddV6(ifName, addr string) error
	SetMTU(ifName string, mtu int) error
}
