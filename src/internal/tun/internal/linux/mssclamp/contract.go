package mssclamp

// Families identifies the IP families that carry traffic through a TUN.
type Families struct {
	IPv4 bool
	IPv6 bool
}

// Contract defines MSS clamping management for TCP SYN packets
// routed through the TunGo TUN interface.
type Contract interface {
	Install(tunName string, families Families) error
	Remove(tunName string) error
}
