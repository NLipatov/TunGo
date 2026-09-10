//go:build darwin

package utun

import (
	"encoding/binary"
	"errors"
	"sync"
	"sync/atomic"

	"golang.org/x/sys/unix"
)

const (
	controlName   = "com.apple.net.utun_control"
	headerLen     = 4
	optIfName     = 2
	ioClosingMask = int64(-1 << 63)

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

	// ioState stores ioClosingMask in the high bit and the number of active
	// I/O operations in the remaining bits.
	ioState   atomic.Int64
	ioDrained chan struct{}
	closeOnce sync.Once
	closeErr  error
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

	return &tun{fd: fd, name: name, ioDrained: make(chan struct{})}, nil
}

func (t *tun) Name() string { return t.name }

// Read fills p with an IP packet without the UTUN address-family header.
func (t *tun) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, errors.New("destination slice too small")
	}

	t.readIOV[0] = t.readHdr[:]
	t.readIOV[1] = p
	if !t.tryAcquireIO() {
		return 0, unix.EBADF
	}
	n, err := unix.Readv(t.fd, t.readIOV[:])
	t.releaseIO()
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
	if !t.tryAcquireIO() {
		return 0, unix.EBADF
	}
	n, err := unix.Writev(t.fd, t.writeIOV[:])
	t.releaseIO()
	if err != nil {
		return 0, err
	}
	if n < headerLen {
		return 0, errors.New("short write (no UTUN header)")
	}
	return len(p), nil
}

func (t *tun) Close() error {
	t.closeOnce.Do(func() {
		activeIO := t.ioState.Or(ioClosingMask)
		if activeIO > 0 {
			// Wake a blocked Readv without releasing the descriptor. Closing it
			// before active operations drain would allow its number to be reused.
			_ = unix.Shutdown(t.fd, unix.SHUT_RD)
			<-t.ioDrained
		}
		t.closeErr = unix.Close(t.fd)
	})
	return t.closeErr
}

func (t *tun) tryAcquireIO() bool {
	for {
		state := t.ioState.Load()
		if state&ioClosingMask != 0 {
			return false
		}
		if t.ioState.CompareAndSwap(state, state+1) {
			return true
		}
	}
}

func (t *tun) releaseIO() {
	shouldNotifyDrained := t.ioState.Add(-1) == ioClosingMask
	if shouldNotifyDrained {
		close(t.ioDrained)
	}
}
