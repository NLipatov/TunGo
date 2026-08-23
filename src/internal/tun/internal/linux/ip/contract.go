package ip

// Contract describes Linux network configuration operations provided through iproute2.
type Contract interface {
	TunTapAddDevTun(devName string) error
	LinkDelete(devName string) error
	LinkSetDevUp(devName string) error
	LinkSetDevMTU(devName string, mtu int) error
	AddrAddDev(devName string, cidr string) error
	RouteDefault() (string, error)
	RouteAddSplitDefaultDev(devName string) error
	Route6AddSplitDefaultDev(devName string) error
	RouteDelSplitDefault(devName string) error
	Route6DelSplitDefault(devName string) error
	RouteGet(hostIp string) (string, error)
	RouteReplaceDev(hostIp string, ifName string) error
	RouteReplaceViaDev(hostIp string, ifName string, gateway string) error
	RouteDel(hostIp string) error
}
