package udp

import "net/netip"

// Packet is a caller-owned packet buffer used by batched UDP ingress.
// Data must be preallocated before ReadBatch; on return it is resliced to the
// received datagram length.
type Packet struct {
	Data  []byte
	Addr  netip.AddrPort
	Flags int
}
