package ip

type Contract interface {
	LinkAddrAdd(ifName, cidr string) error
}
