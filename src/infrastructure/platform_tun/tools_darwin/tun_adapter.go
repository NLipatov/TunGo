package tools_darwin

import (
	"encoding/binary"
	"errors"
	"golang.zx2c4.com/wireguard/tun"
	"syscall"
	"tungo/application"
	"tungo/infrastructure/network"
)

// DarwinWgTunAdapter wraps a wireguard/tun Device and uses **pre‑allocated**
// read / write buffers to avoid per‑packet heap allocations. The first
// 4 bytes of every frame contain the utun address‑family header; Read
// strips it, Write prepends it.
type DarwinWgTunAdapter struct {
	device      tun.Device
	readBuffer  []byte // len == MaxPacketLengthBytes (+4 bytes for header already accounted for by tun.Read offset)
	writeBuffer []byte // ditto
}

// NewWgTunAdapter allocates two reusable buffers large enough to hold the
// biggest frame the VPN ever processes.
func NewWgTunAdapter(dev tun.Device) application.TunDevice {
	return &DarwinWgTunAdapter{
		device:      dev,
		readBuffer:  make([]byte, network.MaxPacketLengthBytes),
		writeBuffer: make([]byte, network.MaxPacketLengthBytes),
	}
}

// Read copies an IP packet from the utun device into p. The utun header is
// skipped, so the caller receives a clean IPv4/IPv6 packet.
func (a *DarwinWgTunAdapter) Read(p []byte) (int, error) {
	bufs, sizes := [][]byte{a.readBuffer}, []int{0}

	// offset 4 tells the driver that the first 4 bytes are the utun header
	if _, err := a.device.Read(bufs, sizes, 4); err != nil {
		return 0, err
	}
	n := sizes[0]
	if n > len(p) {
		return 0, errors.New("destination slice too small")
	}
	copy(p, a.readBuffer[4:4+n])
	return n, nil
}

// Write prepends the utun header to p and writes it to the kernel without
// extra allocations.
func (a *DarwinWgTunAdapter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, errors.New("empty packet")
	}
	if len(p)+4 > len(a.writeBuffer) {
		return 0, errors.New("packet exceeds max size")
	}

	// Determine address family
	var family uint32
	if p[0]>>4 == 6 {
		family = syscall.AF_INET6
	} else {
		family = syscall.AF_INET
	}
	binary.BigEndian.PutUint32(a.writeBuffer[:4], family)
	copy(a.writeBuffer[4:], p)

	if _, err := a.device.Write([][]byte{a.writeBuffer[:len(p)+4]}, 4); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Close closes the underlying utun device.
func (a *DarwinWgTunAdapter) Close() error { return a.device.Close() }
