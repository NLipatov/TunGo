//go:build windows

package netsh

type Contract interface {
	IPSetAddressStatic(interfaceName, ip, mask string) error
	IPSetAddressWithGateway(interfaceName, ip, mask, gateway string, metric int) error
	IPDeleteAddress(interfaceName, interfaceAddress string) error
	IPSetDNS(interfaceName string, dnsServers []string) error
	IPSetMTU(interfaceName string, mtu int) error
	AddRoutePrefix(prefix, interfaceName string, metric int) error
	IPDeleteRoutePrefix(prefix, interfaceName string) error
	IPDeleteDefaultRoute(interfaceName string) error
}
