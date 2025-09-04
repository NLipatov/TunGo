package service

import (
	"net/netip"
)

// test net blocks are defined in RFC 5737 and RFC 3849.
const (
	testNetOne   = "192.0.2.0/24"    // https://datatracker.ietf.org/doc/html/rfc5737#section-3
	testNetTwo   = "198.51.100.0/24" // https://datatracker.ietf.org/doc/html/rfc5737#section-3
	testNetThree = "203.0.113.0/24"  // https://datatracker.ietf.org/doc/html/rfc5737#section-3
	testNetFour  = "2001:db8::/32"   // https://datatracker.ietf.org/doc/html/rfc3849#section-2
)

// FrameDetector is used to distinct service frames(diagnostic frames) from regular frames.
type FrameDetector struct {
	// testNetPrefixes is a list of RFC 5737 networks,
	// which are non-routable address spaces, used for service purposes in TunGo.
	// for more info see: https://datatracker.ietf.org/doc/html/rfc5737#section-3.
	testNetPrefixes [4]netip.Prefix
}

func NewFrameDetector() *FrameDetector {
	return &FrameDetector{
		testNetPrefixes: [4]netip.Prefix{
			netip.MustParsePrefix(testNetOne),
			netip.MustParsePrefix(testNetTwo),
			netip.MustParsePrefix(testNetThree),
			netip.MustParsePrefix(testNetFour),
		},
	}
}

func (f *FrameDetector) HostIsInServiceNetwork(addr netip.Addr) (bool, error) {
	addr = addr.Unmap()
	for _, prefix := range f.testNetPrefixes {
		if prefix.Contains(addr) {
			return true, nil
		}
	}

	return false, nil
}
