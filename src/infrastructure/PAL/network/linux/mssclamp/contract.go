package mssclamp

// Contract defines MSS clamping management for TCP SYN packets
// routed through the TunGo TUN interface.
type Contract interface {
	Install(tunName string) error
	Remove(tunName string) error
}
