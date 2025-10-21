//go:build darwin

package utun

import (
	"errors"
	"golang.org/x/sys/unix"
)

// UTUN is low-level vector I/O interface used by DarwinTunDevice.
type UTUN interface {
	Read(frags [][]byte, sizes []int, offset int) (int, error)
	Write(frags [][]byte, offset int) (int, error)
	Close() error
	Name() (string, error)
}

const (
	uTunControlName = "com.apple.net.utun_control"
	uTunHeaderSize  = 4
	sysProtoControl = 2 // darwin constant
	uTunOptIfName   = 2 // getsockopt -> interface name like "utun3"
)

// rawUTUN implements UTUN directly over the UTUN kernel control socket.
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
	copy(ci.Name[:], uTunControlName)
	if infoErr := unix.IoctlCtlInfo(fd, &ci); infoErr != nil {
		_ = unix.Close(fd)
		return nil, infoErr
	}

	// Unit=0 -> kernel assigns utunN automatically.
	sa := &unix.SockaddrCtl{ID: ci.Id, Unit: 0}
	if connectErr := unix.Connect(fd, sa); connectErr != nil {
		_ = unix.Close(fd)
		return nil, connectErr
	}

	ifName, err := unix.GetsockoptString(fd, sysProtoControl, uTunOptIfName)
	if err != nil {
		_ = unix.Close(fd)
		return nil, err
	}

	return &rawUTUN{fd: fd, name: ifName}, nil
}

func (u *rawUTUN) Name() (string, error) { return u.name, nil }

func (u *rawUTUN) Read(frags [][]byte, sizes []int, offset int) (int, error) {
	if len(frags) == 0 || len(sizes) == 0 {
		return 0, errors.New("invalid args")
	}
	if offset < uTunHeaderSize {
		return 0, errors.New("offset must be >= 4")
	}

	buf := frags[0]
	if len(buf) < offset {
		return 0, errors.New("buffer too small for offset")
	}

	// Kernel returns: [4-byte AF header][IP packet].
	// Read so that header lands at buf[offset-4:offset], payload at buf[offset:].
	n, err := unix.Read(u.fd, buf[offset-uTunHeaderSize:])
	if err != nil {
		return 0, err
	}
	if n < uTunHeaderSize {
		return 0, errors.New("short read (no UTUN header)")
	}
	sizes[0] = n - uTunHeaderSize
	return 1, nil // 1 fragment written
}

func (u *rawUTUN) Write(frags [][]byte, _ int) (int, error) {
	if len(frags) == 0 {
		return 0, errors.New("no buffers")
	}
	// frags[0] must already contain the 4-byte UTUN header.
	n, err := unix.Write(u.fd, frags[0])
	if err != nil {
		return 0, err
	}
	if n < uTunHeaderSize {
		return 0, errors.New("short write (no UTUN header)")
	}
	return n - uTunHeaderSize, nil
}

func (u *rawUTUN) Close() error { return unix.Close(u.fd) }
