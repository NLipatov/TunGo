//go:build windows

package netsh

type Contract interface {
	SetAddressStatic(ifName, ip, mask string) error
	SetAddressWithGateway(ifName, ip, mask, gateway string, metric int) error
	DeleteAddress(ifName, interfaceAddress string) error
	SetDNS(ifName string, dnsServers []string) error
	SetMTU(ifName string, mtu int) error
	AddRoutePrefix(prefix, ifName string, metric int) error
	DeleteRoutePrefix(prefix, ifName string) error
	DeleteDefaultRoute(ifName string) error
	AddHostRouteViaGateway(hostIP, ifName, gateway string, metric int) error
	AddHostRouteOnLink(hostIP, ifName string, metric int) error
	AddDefaultSplitRoutes(ifName string, metric int) error
	DeleteDefaultSplitRoutes(ifName string) error
}
