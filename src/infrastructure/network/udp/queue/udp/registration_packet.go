package udp

import "tungo/infrastructure/settings"

// RegistrationPacket holds a single UDP datagram for a registering client.
// The buffer is pre-allocated and reused, so no per-packet allocations happen
// once the queue is created.
type RegistrationPacket struct {
	n      int
	buffer [settings.DefaultEthernetMTU + settings.UDPChacha20Overhead]byte
}
