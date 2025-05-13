package ip

type Contract interface {
	TunTapAddDevTun(devName string) (string, error)
	LinkDelete(devName string) (string, error)
	LinkSetDevUp(devName string) (string, error)
	LinkSetDevMTU(devName string, mtu int) error
	AddrAddDev(devName string, ip string) (string, error)
	AddrShowDev(ipV int, ifName string) (string, error)
	RouteDefault() (string, error)
	RouteAddDefaultDev(devName string) (string, error)
	RouteGet(hostIp string) (string, error)
	RouteAddDev(hostIp string, ifName string) error
	RouteAddViaDev(hostIp string, ifName string, gateway string) error
	RouteDel(hostIp string) error
}
