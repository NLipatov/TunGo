//go:build linux

// Package epoll provides a TUN wrapper that uses epoll(7) to avoid goroutine-blocking
// read(2)/write(2) calls. It splits readiness into two independent epoll instances:
// one for readability and one for writability. This prevents noisy wake-ups where
// EPOLLOUT is almost always "ready" and would otherwise cause a hot loop while
// waiting for EPOLLIN.
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

// tun wraps a duplicated non-blocking TUN fd and two epoll instances:
// - epIn  watches for EPOLLIN|ERR|HUP (read readiness)
// - epOut watches for EPOLLOUT|ERR|HUP (write readiness)
//
// Concurrency:
// - Read and Write may be called concurrently from different goroutines.
// - Multiple concurrent Reads (or Writes) on the same instance are not supported.
type tun struct {
	fd     int
	epIn   int
	epOut  int
	closed atomic.Bool
}

// Compile-time interface conformance (adjust if your interface differs).
var _ application.Device = (*tun)(nil)

// NewTUN takes ownership of f on success (it closes f before returning).
// On error, ownership remains with the caller (f is not closed).
func NewTUN(f *os.File) (application.Device, error) {
	if f == nil {
		return nil, errors.New("nil file")
	}
	orig := int(f.Fd())

	// 1) Duplicate fd so the wrapper owns lifetime independently of f.
	dup, err := unix.Dup(orig)
	if err != nil {
		return nil, err
	}

	// 2) Make dup non-blocking and close-on-exec.
	if err := unix.SetNonblock(dup, true); err != nil {
		_ = unix.Close(dup)
		return nil, err
	}
	if _, err := unix.FcntlInt(uintptr(dup), unix.F_SETFD, unix.FD_CLOEXEC); err != nil {
		_ = unix.Close(dup)
		return nil, err
	}

	// 3) Create two epoll instances (CLOEXEC).
	epIn, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		_ = unix.Close(dup)
		return nil, err
	}
	epOut, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		_ = unix.Close(epIn)
		_ = unix.Close(dup)
		return nil, err
	}

	// 4) Register the same data fd in both epoll instances with *separate* masks.
	inEv := unix.EpollEvent{
		Events: unix.EPOLLIN | unix.EPOLLERR | unix.EPOLLHUP | unix.EPOLLHUP, // include both HUP bit names for safety
		Fd:     int32(dup),
	}
	if err := unix.EpollCtl(epIn, unix.EPOLL_CTL_ADD, dup, &inEv); err != nil {
		_ = unix.Close(epOut)
		_ = unix.Close(epIn)
		_ = unix.Close(dup)
		return nil, err
	}

	outEv := unix.EpollEvent{
		Events: unix.EPOLLOUT | unix.EPOLLERR | unix.EPOLLHUP | unix.EPOLLHUP,
		Fd:     int32(dup),
	}
	if err := unix.EpollCtl(epOut, unix.EPOLL_CTL_ADD, dup, &outEv); err != nil {
		_ = unix.Close(epOut)
		_ = unix.Close(epIn)
		_ = unix.Close(dup)
		return nil, err
	}

	// 5) Success path: close the original os.File handle; we own dup+epIn+epOut now.
	_ = f.Close()
	runtime.KeepAlive(f) // ensure f is alive across syscalls above

	return &tun{fd: dup, epIn: epIn, epOut: epOut}, nil
}

// Read reads a single TUN packet (or less if p is smaller). On EAGAIN it blocks
// in epoll_wait for readable readiness. Returns io.ErrClosedPipe if closed.
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
		case errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK):
			if err := w.waitRead(); err != nil {
				return 0, err
			}
			continue
		case errors.Is(err, unix.EBADF):
			return 0, io.ErrClosedPipe
		default:
			return 0, err
		}
	}
}

// Write writes one TUN packet. TUN generally expects whole frames, but we still
// handle partial writes conservatively. On EAGAIN it blocks in epoll_wait for EPOLLOUT.
// Returns io.ErrClosedPipe if closed.
func (w *tun) Write(p []byte) (int, error) {
	if w.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	total := 0
	for total < len(p) {
		n, err := unix.Write(w.fd, p[total:])
		if err == nil {
			if n == 0 {
				// Treat as transient backpressure: wait for writable to avoid spinning.
				if err := w.waitWrite(); err != nil {
					return total, err
				}
				continue
			}
			total += n
			continue
		}
		switch {
		case errors.Is(err, unix.EINTR):
			continue
		case errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK):
			if err := w.waitWrite(); err != nil {
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

// Close closes the epoll instances first (to wake any waiters), then the data fd.
// It is safe to call multiple times.
func (w *tun) Close() error {
	if !w.closed.CompareAndSwap(false, true) {
		return nil
	}
	// Close epolls first so blocked epoll_wait calls return with error.
	var firstErr error
	if err := unix.Close(w.epIn); err != nil {
		firstErr = err
	}
	if err := unix.Close(w.epOut); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := unix.Close(w.fd); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

// Fd exposes the underlying duplicated fd (owned by this wrapper). Use with care.
func (w *tun) Fd() uintptr { return uintptr(w.fd) }

// waitRead blocks until the fd is readable, or returns io.EOF on HUP/ERR,
// or io.ErrClosedPipe if the wrapper is closed.
func (w *tun) waitRead() error {
	var evs [1]unix.EpollEvent
	for {
		n, err := unix.EpollWait(w.epIn, evs[:], -1)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			// If epoll fd is closed concurrently, translate EBADF to closed pipe.
			if errors.Is(err, unix.EBADF) || w.closed.Load() {
				return io.ErrClosedPipe
			}
			return err
		}
		if n <= 0 {
			// With timeout -1 this should not happen; loop defensively.
			continue
		}
		ev := evs[0].Events
		if (ev & (unix.EPOLLERR | unix.EPOLLHUP | unix.EPOLLHUP)) != 0 {
			return io.EOF
		}
		if (ev & unix.EPOLLIN) != 0 {
			return nil
		}
		// Ignore unrelated events (shouldn't occur with split epoll), and loop.
	}
}

// waitWrite blocks until the fd is writable, or returns io.EOF on HUP/ERR,
// or io.ErrClosedPipe if the wrapper is closed.
func (w *tun) waitWrite() error {
	var evs [1]unix.EpollEvent
	for {
		n, err := unix.EpollWait(w.epOut, evs[:], -1)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			// If epoll fd is closed concurrently, translate EBADF to closed pipe.
			if errors.Is(err, unix.EBADF) || w.closed.Load() {
				return io.ErrClosedPipe
			}
			return err
		}
		if n <= 0 {
			// With timeout -1 this should not happen; loop defensively.
			continue
		}
		ev := evs[0].Events
		if (ev & (unix.EPOLLERR | unix.EPOLLHUP | unix.EPOLLHUP)) != 0 {
			return io.EOF
		}
		if (ev & unix.EPOLLOUT) != 0 {
			return nil
		}
		// Ignore unrelated events (shouldn't occur with split epoll), and loop.
	}
}
