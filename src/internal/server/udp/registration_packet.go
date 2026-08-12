package udp

import "tungo/internal/config/settings"

// registrationPacket holds a single UDP datagram for a registering client.
// The buffer is pre-allocated and reused, so no per-packet allocations happen
// once the queue is created.
type registrationPacket struct {
	n      int
	buffer [settings.DefaultEthernetMTU + settings.UDPChacha20Overhead]byte
}
