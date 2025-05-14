package netsh

type Contract interface {
	RouteDelete(hostIP string) error
	InterfaceIPSetAddressStatic(interfaceName, hostIP, mask, gateway string) error
	InterfaceIPV4AddRouteDefault(interfaceName, gateway string) error
	InterfaceIPV4DeleteAddress(IfName string) error
	InterfaceIPDeleteAddress(IfName, IfAddr string) error
	SetInterfaceMetric(interfaceName string, metric int) error
}
