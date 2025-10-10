package transport

// Handler handles packages from Transport(UDP or TCP) to TUN
type Handler interface {
	HandleTransport() error
}
