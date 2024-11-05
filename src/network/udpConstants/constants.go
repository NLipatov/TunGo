package udpConstants

const (
	// MaxDatagramSize RFC 768: max UDP datagram size is 65 535 bytes.
	MaxDatagramSize = 65_535

	// MaxTunPacketSize guarantees that UDP TunGo's packet will not exceed 65_507
	// (65_507 bytes is max datagram size minus IP and headers).
	// Max TunGo client transmittable UDP packet will be: 65_535 - 8 - 20 - 12 = 65495 bytes, where:
	// - 65_535 - max UDP datagram;
	// - 8 bytes - UDP headers;
	// - 20 bytes - IP;
	// - 12 bytes - nonce;
	MaxTunPacketSize = MaxDatagramSize - 8 - 20 - 12

	// MaxUdpPacketSize is length of MaxTunPacketSize + 12 bytes - because we need large enough buffer to read TunPacket and additional 12 bytes of nonce.
	MaxUdpPacketSize = MaxTunPacketSize + 12
)
