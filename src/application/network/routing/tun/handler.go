package tun

// Handler handles packages from TUN to Transport(UDP or TCP)
type Handler interface {
	HandleTun() error
}
