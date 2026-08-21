//go:build darwin

package utun

import (
	"encoding/binary"
	"errors"

	"golang.org/x/sys/unix"
)

const (
	controlName = "com.apple.net.utun_control"
	headerLen   = 4
	optIfName   = 2

	// Darwin's SYSPROTO_CONTROL numeric value. Some Go builds don't export it;
	// the ABI value is stable on Darwin.
	sysProtoControl = 2
)

type tun struct {
	fd   int
	name string

	readHdr  [headerLen]byte
	readIOV  [2][]byte
	writeHdr [headerLen]byte
	writeIOV [2][]byte
}

func New() (*tun, error) {
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

	if err := unix.Connect(fd, &unix.SockaddrCtl{ID: ci.Id, Unit: 0}); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}

	name, err := unix.GetsockoptString(fd, sysProtoControl, optIfName)
	if err != nil {
		_ = unix.Close(fd)
		return nil, err
	}

	return &tun{fd: fd, name: name}, nil
}

func (t *tun) Name() string { return t.name }

// Read fills p with an IP packet without the UTUN address-family header.
func (t *tun) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, errors.New("destination slice too small")
	}

	t.readIOV[0] = t.readHdr[:]
	t.readIOV[1] = p
	n, err := unix.Readv(t.fd, t.readIOV[:])
	if err != nil {
		return 0, err
	}
	if n < headerLen {
		return 0, errors.New("short read (no UTUN header)")
	}
	return n - headerLen, nil
}

// Write sends p with the UTUN address-family header for its IP version.
func (t *tun) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, errors.New("empty packet")
	}

	af := unix.AF_INET
	if p[0]>>4 == 6 {
		af = unix.AF_INET6
	}
	binary.BigEndian.PutUint32(t.writeHdr[:], uint32(af))

	t.writeIOV[0] = t.writeHdr[:]
	t.writeIOV[1] = p
	n, err := unix.Writev(t.fd, t.writeIOV[:])
	if err != nil {
		return 0, err
	}
	if n < headerLen {
		return 0, errors.New("short write (no UTUN header)")
	}
	return len(p), nil
}

func (t *tun) Close() error {
	if err := unix.Close(t.fd); err != nil {
		return err
	}
	t.fd = -1
	return nil
}
