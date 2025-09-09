package application

import (
	"time"
)

// MTUProber defines interface for probing MTU sizes.
// Implementations should send a probe frame of the requested size and
// report whether the peer acknowledged it. RTT of the probe may be
// returned for logging/metrics purposes.
type MTUProber interface {
	// SendProbe transmits a probe frame with payload of the given size.
	SendProbe(size int) error
	// AwaitAck waits for acknowledgement of the last probe until timeout.
	// It returns true if the probe was acknowledged along with measured RTT.
	AwaitAck(timeout time.Duration) (bool, time.Duration, error)
}

// DiscoverMTU performs a binary search using the provided prober to find the
// maximum payload size that successfully reaches the peer.
// The search is performed in the range [min,max] and returns the best value
// that was acknowledged. If the probing fails to send or receive an
// acknowledgement, the current best value is returned along with the error.
func DiscoverMTU(p MTUProber, min, max int, timeout time.Duration) (int, error) {
	low, high := min, max
	best := min

	for low <= high {
		mid := (low + high) / 2
		if err := p.SendProbe(mid); err != nil {
			return best, err
		}
		ok, _, err := p.AwaitAck(timeout)
		if err != nil {
			return best, err
		}
		if ok {
			best = mid
			low = mid + 1
		} else {
			high = mid - 1
		}
	}
	return best, nil
}
