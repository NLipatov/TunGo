//go:build darwin

package utun

import (
	"encoding/binary"
	"errors"

	"golang.org/x/sys/unix"
	"tungo/application/network/routing/tun"
)

// DarwinTunDevice adapts UTUN to the portable tun.Device API.
// It uses vectored I/O ([hdr(4), payload]) with preallocated iovecs to avoid heap allocs.
type DarwinTunDevice struct {
	device UTUN

	// Preallocated scatter/gather vectors & scratch for READ path.
	readHdr   [4]byte   // AF header sink for readv
	readIOV   [2][]byte // [hdr, payload]
	readSizes [1]int    // sizes scratch for UTUN.Read

	// Preallocated scatter/gather vectors & scratch for WRITE path.
	writeHdr   [4]byte   // AF header source for writev
	writeIOV   [2][]byte // [hdr, payload]
	writeSizes [1]int    // (not used by write, but kept symmetric)
}

func NewDarwinTunDevice(dev UTUN) tun.Device {
	return &DarwinTunDevice{device: dev}
}

// Read fills p with a clean IP packet (without the 4-byte UTUN header).
// No heap allocations in the hot path.
func (a *DarwinTunDevice) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, errors.New("destination slice too small")
	}
	a.readIOV[0] = a.readHdr[:]
	a.readIOV[1] = p
	a.readSizes[0] = 0

	if _, err := a.device.Read(a.readIOV[:], a.readSizes[:], 0); err != nil {
		return 0, err
	}
	return a.readSizes[0], nil
}

// Write sends p by prepending the 4-byte UTUN AF header (IPv4/IPv6).
// No heap allocations in the hot path.
func (a *DarwinTunDevice) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, errors.New("empty packet")
	}

	af := unix.AF_INET
	if p[0]>>4 == 6 {
		af = unix.AF_INET6
	}
	binary.BigEndian.PutUint32(a.writeHdr[:], uint32(af))

	a.writeIOV[0] = a.writeHdr[:]
	a.writeIOV[1] = p

	if _, err := a.device.Write(a.writeIOV[:], 0); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (a *DarwinTunDevice) Close() error { return a.device.Close() }
