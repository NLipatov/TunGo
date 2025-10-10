package tun

// Worker does the TUN->CONN and CONN->TUN operations
type Worker interface {
	// HandleTun handles packets from TUN-like interface
	HandleTun() error
	// HandleTransport handles packets from transport connection
	HandleTransport() error
}
