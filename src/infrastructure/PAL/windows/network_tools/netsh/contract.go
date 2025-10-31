//go:build windows

package netsh

type Contract interface {
	// InterfaceSetAddressNoGateway IP address on interface without a gateway (on-link).
	InterfaceSetAddressNoGateway(interfaceName, ip, mask string) error
	// InterfaceSetAddressWithGateway IP address on interface with an explicit next-hop gateway.
	InterfaceSetAddressWithGateway(interfaceName, ip, mask, gateway string, metric int) error
	RouteDelete(destinationIP string) error
	InterfaceIPDeleteAddress(interfaceName, interfaceAddress string) error
	SetInterfaceMetric(interfaceName string, metric int) error
	InterfaceSetDNSServers(interfaceName string, dnsServers []string) error
	LinkSetDevMTU(interfaceName string, mtu int) error
	InterfaceAddRouteOnLink(prefix, interfaceName string, metric int) error
	InterfaceDeleteRoute(prefix, interfaceName string) error
	InterfaceDeleteDefaultRoute(interfaceName string) error
}
