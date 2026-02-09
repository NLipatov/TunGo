package settings

import "net/netip"

type Settings struct {
	InterfaceName    string       `json:"InterfaceName"`
	InterfaceSubnet  netip.Prefix `json:"InterfaceSubnet"`
	InterfaceIP      netip.Addr   `json:"InterfaceIP"`
	IPv6Subnet       netip.Prefix `json:"IPv6Subnet,omitempty"`
	IPv6IP           netip.Addr   `json:"IPv6IP,omitempty"`
	Host             Host         `json:"Host"`
	IPv6Host         Host         `json:"IPv6Host,omitempty"`
	Port             int          `json:"Port"`
	MTU              int          `json:"MTU"`
	Protocol         Protocol
	Encryption       Encryption
	DialTimeoutMs    DialTimeoutMs `json:"DialTimeoutMs"`
}
