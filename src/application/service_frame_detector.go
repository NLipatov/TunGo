package application

import "net/netip"

// DocNetDetector is used to distinct service frames(diagnostic frames) from regular frames.
type DocNetDetector interface {
	IsInDocNet(addr netip.Addr) bool
}
