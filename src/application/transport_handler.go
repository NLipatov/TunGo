package application

// TransportHandler handles packages from Transport(UDP or TCP) to TUN
type TransportHandler interface {
	HandleTransport() error
}
