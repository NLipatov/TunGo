//go:build windows

package netsh

type Contract interface {
	// InterfaceIPv4SetAddressNoGateway IPv4 address on interface without a gateway (on-link).
	InterfaceIPv4SetAddressNoGateway(interfaceName, ip, mask string) error
	// InterfaceIPv4SetAddressWithGateway IPv4 address on interface with an explicit next-hop gateway.
	InterfaceIPv4SetAddressWithGateway(interfaceName, ip, mask, gateway string, metric int) error
	RouteDelete(destinationIP string) error
	InterfaceIPDeleteAddress(interfaceName, interfaceAddress string) error
	SetInterfaceMetric(interfaceName string, metric int) error
	InterfaceSetDNSServers(interfaceName string, dnsServers []string) error
	LinkSetDevMTU(interfaceName string, mtu int) error
	InterfaceIPV4AddRouteOnLink(prefix, interfaceName string, metric int) error
	InterfaceIPV4DeleteRoute(prefix, interfaceName string) error
	InterfaceIPV4DeleteDefaultRoute(interfaceName string) error
}
