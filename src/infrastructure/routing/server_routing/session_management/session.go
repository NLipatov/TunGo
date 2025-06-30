package session_management

import "net/netip"

type ClientSession interface {
	ExternalIP() netip.AddrPort
	InternalIP() netip.Addr
}
