package network

type IPHeader interface {
	ReadDestinationAddressBytes(buffer []byte) error
}
