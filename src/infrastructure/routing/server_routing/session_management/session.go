package session_management

import "net/netip"

type ClientSession interface {
	ExternalIP() netip.Addr
	InternalIP() netip.Addr
}
