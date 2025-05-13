package ip

// Contract is a interface of a wrapper around ip command from the iproute2 tool collection
type Contract interface {
	TunTapAddDevTun(devName string) error
	LinkDelete(devName string) error
	LinkSetDevUp(devName string) error
	LinkSetDevMTU(devName string, mtu int) error
	AddrAddDev(devName string, ip string) error
	AddrShowDev(ipV int, ifName string) (string, error)
	RouteDefault() (string, error)
	RouteAddDefaultDev(devName string) error
	RouteGet(hostIp string) (string, error)
	RouteAddDev(hostIp string, ifName string) error
	RouteAddViaDev(hostIp string, ifName string, gateway string) error
	RouteDel(hostIp string) error
}
