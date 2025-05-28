package application

// TunHandler handles packages from TUN to Transport(UDP or TCP)
type TunHandler interface {
	HandleTun() error
}
