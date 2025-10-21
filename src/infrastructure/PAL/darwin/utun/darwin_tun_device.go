//go:build darwin

package utun

import (
	"encoding/binary"
	"errors"
	"syscall"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/settings"
)

const headerSize = 4

// DarwinTunDevice is Darwin-specific implementation of tun.Device.
type DarwinTunDevice struct {
	device UTUN

	readBuffer  []byte // backing array for incoming packets (+4 bytes hdr)
	writeBuffer []byte // backing array for outgoing packets (+4 bytes hdr)

	// Pre‑built slice headers reused on every Read/Write call.
	readVec  [][]byte // len==1, always points to readBuffer
	writeVec [][]byte // len==1, resliced to current packet size
	sizes    []int    // len==1, scratch for Device.Read
}

// NewDarwinTunDevice allocates the buffers once and prepares reusable slice
// headers. MaxPacketLengthBytes should already include the 4‑byte utun header.
func NewDarwinTunDevice(dev UTUN) tun.Device {
	rb := make([]byte, settings.DefaultEthernetMTU+headerSize)
	wb := make([]byte, settings.DefaultEthernetMTU+headerSize)
	return &DarwinTunDevice{
		device:      dev,
		readBuffer:  rb,
		writeBuffer: wb,
		readVec:     [][]byte{rb},
		writeVec:    [][]byte{wb}, // resliced per packet
		sizes:       []int{0},
	}
}

// Read copies a clean IP packet (without the 4‑byte utun header) into p.
// No heap allocations occur.
func (a *DarwinTunDevice) Read(p []byte) (int, error) {
	a.sizes[0] = 0 // reset size slot

	// offset=4: driver writes after utun header
	if _, err := a.device.Read(a.readVec, a.sizes, 4); err != nil {
		return 0, err
	}
	n := a.sizes[0]
	if n > len(p) {
		return 0, errors.New("destination slice too small")
	}
	copy(p, a.readBuffer[4:4+n])
	return n, nil
}

// Write prepends utun header and transmits p without allocations.
func (a *DarwinTunDevice) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, errors.New("empty packet")
	}
	if len(p)+4 > len(a.writeBuffer) {
		return 0, errors.New("packet exceeds max size")
	}

	// Address family from first nibble of IP header
	var family uint32
	if p[0]>>4 == 6 {
		family = syscall.AF_INET6
	} else {
		family = syscall.AF_INET
	}
	binary.BigEndian.PutUint32(a.writeBuffer[:4], family)
	copy(a.writeBuffer[4:], p)

	// Re‑slice reusable header to actual packet length (+4 hdr)
	a.writeVec[0] = a.writeBuffer[:len(p)+4]

	if _, err := a.device.Write(a.writeVec, 4); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Close closes the underlying utun device.
func (a *DarwinTunDevice) Close() error { return a.device.Close() }
