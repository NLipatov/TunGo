package application

import "net/netip"

// ServiceFrameDetector is used to distinct service frames(diagnostic frames) from regular frames.
type ServiceFrameDetector interface {
	HostIsInServiceNetwork(addr netip.Addr) bool
}
