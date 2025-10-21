//go:build darwin

package utun

import (
	"errors"

	"golang.org/x/sys/unix"
)

const (
	// Kernel control name for UTUN.
	controlName = "com.apple.net.utun_control"

	// getsockopt(level=sysProtoControl, optname=optIfName) returns "utunN".
	optIfName = 2

	// Size of the 4-byte UTUN address-family header (AF_INET/AF_INET6).
	headerLen = 4

	// Darwin's SYSPROTO_CONTROL numeric value.
	// Some Go builds don't export it; the ABI value is stable (2) on darwin.
	sysProtoControl = 2
)

// UTUN is a low-level vectored I/O interface over the UTUN kernel control socket.
// Contract (vectored-only):
//   - Read/Write MUST be called with len(frags) >= 2 and offset == 0.
//   - frags[0] is the 4-byte header buffer (>= 4 bytes).
//   - frags[1] is the IP payload buffer.
type UTUN interface {
	Read(frags [][]byte, sizes []int, offset int) (int, error)
	Write(frags [][]byte, offset int) (int, error)
	Close() error
	Name() (string, error)
}

// rawUTUN implements UTUN on top of AF_SYSTEM/SYSPROTO_CONTROL socket.
type rawUTUN struct {
	fd   int
	name string
}

func newRawUTUN() (*rawUTUN, error) {
	fd, err := unix.Socket(unix.AF_SYSTEM, unix.SOCK_DGRAM, sysProtoControl)
	if err != nil {
		return nil, err
	}

	var ci unix.CtlInfo
	copy(ci.Name[:], controlName)
	if err := unix.IoctlCtlInfo(fd, &ci); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}

	// Unit=0 → kernel picks the next available utunN.
	sa := &unix.SockaddrCtl{ID: ci.Id, Unit: 0}
	if err := unix.Connect(fd, sa); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}

	// Query interface name ("utunN") from the control socket.
	ifName, err := unix.GetsockoptString(fd, sysProtoControl, optIfName)
	if err != nil {
		_ = unix.Close(fd)
		return nil, err
	}

	return &rawUTUN{fd: fd, name: ifName}, nil
}

func (u *rawUTUN) Name() (string, error) { return u.name, nil }

// Read performs a single UTUN datagram read with scatter I/O:
//   - frags[0][:4] receives the 4-byte AF header
//   - frags[1]     receives the IP payload
//
// sizes[0] is set to the payload length (excluding the 4-byte header).
func (u *rawUTUN) Read(frags [][]byte, sizes []int, offset int) (int, error) {
	if len(sizes) == 0 {
		return 0, errors.New("sizes required")
	}
	if offset != 0 {
		return 0, errors.New("offset must be 0 in vectored mode")
	}
	if len(frags) < 2 {
		return 0, errors.New("need two buffers: hdr and payload")
	}
	hdr := frags[0]
	payload := frags[1]
	if len(hdr) < headerLen {
		return 0, errors.New("hdr too small (<4)")
	}

	// Limit the first iovec to exactly 4 bytes, so kernel writes only AF header there.
	n, err := unix.Readv(u.fd, [][]byte{hdr[:headerLen], payload})
	if err != nil {
		return 0, err
	}
	if n < headerLen {
		return 0, errors.New("short read (no UTUN header)")
	}
	sizes[0] = n - headerLen
	return 2, nil
}

// Write performs a single UTUN datagram write with gather I/O:
//   - frags[0][:4] must contain the AF header
//   - frags[1]     is the IP payload to send
//
// The returned int is the payload byte count (header excluded).
func (u *rawUTUN) Write(frags [][]byte, _ int) (int, error) {
	if len(frags) < 2 {
		return 0, errors.New("need two buffers: hdr and payload")
	}
	hdr := frags[0]
	payload := frags[1]
	if len(hdr) < headerLen {
		return 0, errors.New("hdr too small (<4)")
	}

	n, err := unix.Writev(u.fd, [][]byte{hdr[:headerLen], payload})
	if err != nil {
		return 0, err
	}
	if n < headerLen {
		return 0, errors.New("short write (no UTUN header)")
	}
	return n - headerLen, nil
}

func (u *rawUTUN) Close() error { return unix.Close(u.fd) }
