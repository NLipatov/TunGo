package application

// TunWorker does the TUN->CONN and CONN->TUN operations
type TunWorker interface {
	// HandleTun handles packets from TUN-like interface
	HandleTun() error
	// HandleTransport handles packets from transport connection
	HandleTransport() error
}
