package udpConstants

const (
	standardMTU   = 1500
	UDPHeaderSize = 8
	IPHeaderSize  = 20
	NonceSize     = 12
	TotalOverhead = UDPHeaderSize + IPHeaderSize + NonceSize

	MaxTunPacketSize = standardMTU - TotalOverhead

	// MaxUdpPacketSize is 1472 bytes. Length of 1472 includes 12 bytes of ChaCha20 nonce.
	MaxUdpPacketSize = MaxTunPacketSize + NonceSize
)
