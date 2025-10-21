//go:build darwin

package utun

import (
	"encoding/binary"
	"errors"

	"golang.org/x/sys/unix"
	"tungo/application/network/routing/tun"
)

// DarwinTunDevice adapts UTUN to the portable tun.Device API.
// It always uses vectored I/O ([hdr(4), payload]) to avoid extra user-space copies.
type DarwinTunDevice struct{ device UTUN }

func NewDarwinTunDevice(dev UTUN) tun.Device { return &DarwinTunDevice{device: dev} }

// Read fills p with a clean IP packet (without the 4-byte UTUN header).
// It returns the payload length copied into p.
func (a *DarwinTunDevice) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, errors.New("destination slice too small")
	}
	var hdr [4]byte
	sizes := []int{0}
	if _, err := a.device.Read([][]byte{hdr[:], p}, sizes, 0); err != nil {
		return 0, err
	}
	return sizes[0], nil
}

// Write sends p by prepending the 4-byte UTUN AF header (IPv4/IPv6).
// It returns the payload length sent (header excluded).
func (a *DarwinTunDevice) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, errors.New("empty packet")
	}
	var hdr [4]byte
	af := unix.AF_INET
	if p[0]>>4 == 6 {
		af = unix.AF_INET6
	}
	binary.BigEndian.PutUint32(hdr[:], uint32(af))
	if _, err := a.device.Write([][]byte{hdr[:], p}, 0); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (a *DarwinTunDevice) Close() error { return a.device.Close() }
