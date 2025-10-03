package ifconfig

type Contract interface {
	LinkAddrAdd(ifName, cidr string) error
	SetMTU(ifName string, mtu int) error
}
