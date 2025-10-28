//go:build linux

package epoll

import (
	"errors"
	"io"
	"os"
	"runtime"
	"sync/atomic"
	application "tungo/application/network/routing/tun"

	"golang.org/x/sys/unix"
)

// tun wraps a TUN file descriptor and performs blocking Read/Write
// via epoll(7) instead of blocking read(2)/write(2) on the fd.
// It keeps the same surface API: Read([]byte) (int, error), Write([]byte) (int, error), Close() error.
// Optionally it exposes Fd() for cases where you need the raw fd (e.g., further ioctls).
type tun struct {
	fd      int // duplicated and owned by this wrapper
	epollFd int // epoll instance fd
	closed  atomic.Bool

	// single-entry events array to avoid allocations
	events [1]unix.EpollEvent
}

// NewTUN takes ownership of the given *os.File by duplicating its
// fd (so the caller can safely discard/close the original file). The wrapper
// sets O_NONBLOCK and registers the fd in epoll for level-triggered I/O.
func NewTUN(f *os.File) (application.Device, error) {
	if f == nil {
		return nil, errors.New("nil file")
	}
	orig := int(f.Fd())

	// Duplicate fd so we fully control its lifetime, independent of f.
	dup, err := unix.Dup(orig)
	if err != nil {
		return nil, err
	}
	// The caller may still hold 'f'; duplicating prevents double-close hazards.
	// We do NOT close 'f' here — caller decides. We only own 'dup'.

	// Put the duplicated fd in non-blocking mode so read/write can return EAGAIN
	// which we will bridge with epoll_wait.
	if err := unix.SetNonblock(dup, true); err != nil {
		_ = unix.Close(dup)
		return nil, err
	}

	ep, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		_ = unix.Close(dup)
		return nil, err
	}

	w := &tun{fd: dup, epollFd: ep}
	ev := unix.EpollEvent{
		Events: unix.EPOLLIN | unix.EPOLLOUT | unix.EPOLLERR | unix.EPOLLHUP,
		Fd:     int32(w.fd),
	}
	if err := unix.EpollCtl(w.epollFd, unix.EPOLL_CTL_ADD, w.fd, &ev); err != nil {
		_ = unix.Close(w.epollFd)
		_ = unix.Close(w.fd)
		return nil, err
	}

	// Keep f alive across syscalls above (defensive; fd already dup'ed).
	runtime.KeepAlive(f)
	return w, nil
}

// Read reads a single TUN packet (or less if buffer is smaller) using epoll
// to avoid blocking the goroutine on read(2). On EAGAIN it waits for EPOLLIN.
func (w *tun) Read(p []byte) (int, error) {
	if w.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	for {
		n, err := unix.Read(w.fd, p)
		if err == nil {
			return n, nil
		}
		switch {
		case errors.Is(err, unix.EINTR):
			continue
		case errors.Is(err, unix.EAGAIN), errors.Is(err, unix.EWOULDBLOCK):
			if waitErr := w.wait(unix.EPOLLIN); waitErr != nil {
				return 0, waitErr
			}
			continue
		case errors.Is(err, unix.EBADF):
			return 0, io.ErrClosedPipe
		default:
			// EPOLLHUP/ERR are surfaced via wait(); direct read errors bubble up here.
			return 0, err
		}
	}
}

// Write writes one TUN packet. TUN usually expects whole packets, but we still
// handle partial writes conservatively. On EAGAIN it waits for EPOLLOUT.
func (w *tun) Write(p []byte) (int, error) {
	if w.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	total := 0
	for total < len(p) {
		n, err := unix.Write(w.fd, p[total:])
		if err == nil {
			if n == 0 {
				// Shouldn't happen for non-zero p, treat as EAGAIN-ish spin to avoid tight loop.
				if waitErr := w.wait(unix.EPOLLOUT); waitErr != nil {
					return total, waitErr
				}
				continue
			}
			total += n
			continue
		}
		switch {
		case errors.Is(err, unix.EINTR):
			continue
		case errors.Is(err, unix.EAGAIN), errors.Is(err, unix.EWOULDBLOCK):
			if err := w.wait(unix.EPOLLOUT); err != nil {
				return total, err
			}
			continue
		case errors.Is(err, unix.EBADF):
			return total, io.ErrClosedPipe
		default:
			return total, err
		}
	}
	return total, nil
}

// Close closes both the epoll instance and the owned duplicated fd.
func (w *tun) Close() error {
	if !w.closed.CompareAndSwap(false, true) {
		return nil
	}
	if err := unix.Close(w.epollFd); err != nil {
		return err
	}
	if err := unix.Close(w.fd); err != nil {
		return err
	}
	return nil
}

// Fd returns the underlying fd (owned by this wrapper). Use with care.
func (w *tun) Fd() uintptr { return uintptr(w.fd) }

// wait blocks in epoll_wait until the desired events occur, or returns EOF on HUP/ERR.
func (w *tun) wait(mask uint32) error {
	for {
		n, err := unix.EpollWait(w.epollFd, w.events[:], -1)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		if n == 0 {
			continue
		}
		ev := w.events[0].Events
		if (ev & (unix.EPOLLERR | unix.EPOLLHUP)) != 0 {
			return io.EOF
		}
		if (ev & mask) != 0 {
			return nil
		}
		// Otherwise, loop again (level-triggered).
	}
}
