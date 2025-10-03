// internal/PAL/darwin/tun_adapters/utun_darwin.go
//go:build darwin

package tun_adapters

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"

	"golang.org/x/sys/unix"
)

const (
	utunControlName  = "com.apple.net.utun_control"
	utunHeaderSize   = 4
	SYSPROTO_CONTROL = 2 // darwin constant
	UTUN_OPT_IFNAME  = 2 // getsockopt -> interface name like "utun3"
)

// Adapter is your low-level vector I/O interface used by WgTunAdapter.
type Adapter interface {
	Read(frags [][]byte, sizes []int, offset int) (int, error)
	Write(frags [][]byte, offset int) (int, error)
	Close() error
	Name() (string, error)
}

// UTun implements Adapter directly over the UTUN kernel control socket.
type UTun struct {
	fd   int
	name string
}

func openUTun() (*UTun, error) {
	fd, err := unix.Socket(unix.AF_SYSTEM, unix.SOCK_DGRAM, SYSPROTO_CONTROL)
	if err != nil {
		return nil, err
	}

	var ci unix.CtlInfo
	copy(ci.Name[:], []byte(utunControlName))
	if err := unix.IoctlCtlInfo(fd, &ci); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}

	// Unit=0 -> kernel assigns utunN automatically.
	sa := &unix.SockaddrCtl{ID: ci.Id, Unit: 0}
	if err := unix.Connect(fd, sa); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}

	ifName, err := unix.GetsockoptString(fd, SYSPROTO_CONTROL, UTUN_OPT_IFNAME)
	if err != nil {
		_ = unix.Close(fd)
		return nil, err
	}

	return &UTun{fd: fd, name: ifName}, nil
}

func (u *UTun) Name() (string, error) { return u.name, nil }

func (u *UTun) Read(frags [][]byte, sizes []int, offset int) (int, error) {
	if len(frags) == 0 || len(sizes) == 0 {
		return 0, errors.New("invalid args")
	}
	if offset < utunHeaderSize {
		return 0, errors.New("offset must be >= 4")
	}

	buf := frags[0]
	if len(buf) < offset {
		return 0, errors.New("buffer too small for offset")
	}

	// Kernel returns: [4-byte AF header][IP packet].
	// Read so that header lands at buf[offset-4:offset], payload at buf[offset:].
	n, err := unix.Read(u.fd, buf[offset-utunHeaderSize:])
	if err != nil {
		return 0, err
	}
	if n < utunHeaderSize {
		return 0, errors.New("short read (no UTUN header)")
	}
	sizes[0] = n - utunHeaderSize
	return 1, nil // 1 fragment written
}

func (u *UTun) Write(frags [][]byte, _ int) (int, error) {
	if len(frags) == 0 {
		return 0, errors.New("no buffers")
	}
	// frags[0] must already contain the 4-byte UTUN header.
	n, err := unix.Write(u.fd, frags[0])
	if err != nil {
		return 0, err
	}
	if n < utunHeaderSize {
		return 0, errors.New("short write (no UTUN header)")
	}
	return n - utunHeaderSize, nil
}

func (u *UTun) Close() error { return unix.Close(u.fd) }

// setMTUIfRequested applies MTU via ifconfig (simplest and robust on macOS).
func setMTUIfRequested(ifName string, mtu int) error {
	if mtu <= 0 {
		return nil
	}
	cmd := exec.Command("ifconfig", ifName, "mtu", strconv.Itoa(mtu))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ifconfig set mtu failed: %w; output: %s", err, string(out))
	}
	return nil
}

// CreateTUN mimics the API of wireguard/tun.CreateTUN on darwin.
// Note: macOS ignores the requested 'name' and always assigns utunN automatically.
// We still accept 'name' to keep the signature familiar.
func CreateTUN(name string, mtu int) (Adapter, error) {
	u, err := openUTun()
	if err != nil {
		return nil, err
	}
	ifName := u.name

	// Best-effort MTU setup (same spirit as wg's CreateTUN(mtu)).
	if err := setMTUIfRequested(ifName, mtu); err != nil {
		_ = u.Close()
		return nil, err
	}

	return u, nil
}
