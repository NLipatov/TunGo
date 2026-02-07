package settings

import "net/netip"

type Settings struct {
	InterfaceName    string       `json:"InterfaceName"`
	InterfaceSubnet  netip.Prefix `json:"InterfaceSubnet"`
	InterfaceIP      netip.Addr   `json:"InterfaceIP"`
	Host             Host         `json:"Host"`
	Port             int          `json:"Port"`
	MTU              int          `json:"MTU"`
	Protocol         Protocol
	Encryption       Encryption
	DialTimeoutMs    DialTimeoutMs `json:"DialTimeoutMs"`
}
