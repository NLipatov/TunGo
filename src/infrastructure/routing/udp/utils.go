package udp

func IsMulticastPacket(payload []byte) bool {
	if len(payload) < 1 {
		return false
	}
	// IPv6 multicast ff00::/8 — check first byte == 0xff
	if payload[0] == 0xff {
		return true
	}
	// IPv4 multicast 224.0.0.0/4 — check first byte 0xe0..0xef
	if b := payload[0]; b >= 0xe0 && b <= 0xef {
		return true
	}
	// Optionally, if we had dest address we could check netip; here we rely on first byte.
	return false
}
