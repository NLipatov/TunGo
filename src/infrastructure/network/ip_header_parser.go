package network

type IPHeader interface {
	// ParseDestinationAddressBytes reads 32 bits of 'Destination Address' into given buffer
	ParseDestinationAddressBytes(header, resultBuffer []byte) error
}
