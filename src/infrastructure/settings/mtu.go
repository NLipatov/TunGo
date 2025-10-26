package settings

func ResolveMTU(mtu int) int {
	if mtu <= 0 {
		return DefaultEthernetMTU
	}
	return mtu
}

func UDPBufferSize(mtu int) int {
	return ResolveMTU(mtu) + UDPChacha20Overhead
}
