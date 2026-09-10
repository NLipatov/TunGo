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
	"sync"

	"tungo/internal/tun/internal/iolifecycle"

	"golang.org/x/sys/unix"
)

// tun wraps a duplicated non-blocking TUN fd and two epoll instances:
// - epIn  watches for EPOLLIN|ERR|HUP (read readiness)
// - epOut watches for EPOLLOUT|ERR|HUP (write readiness)
//
// Concurrency:
// - Read and Write may be called concurrently from different goroutines.
// - Multiple concurrent Reads (or multiple concurrent Writes) on the same instance are NOT supported.
type tun struct {
	fd    int
	epIn  int
	epOut int

	io        iolifecycle.Gate
	closeOnce sync.Once
	closeErr  error
}

// A finite timeout lets Close stop idle operations without a separate wake-up fd.
const epollWaitTimeoutMillis = 250

// New takes ownership of f on success (it closes f before returning).
// On error, ownership remains with the caller (f is not closed).
func New(f *os.File) (io.ReadWriteCloser, error) {
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

	// 4) Register the same data fd in both epoll instances with separate masks.
	inEv := unix.EpollEvent{
		Events: unix.EPOLLIN | unix.EPOLLERR | unix.EPOLLHUP,
		Fd:     int32(dup),
	}
	if err := unix.EpollCtl(epIn, unix.EPOLL_CTL_ADD, dup, &inEv); err != nil {
		_ = unix.Close(epOut)
		_ = unix.Close(epIn)
		_ = unix.Close(dup)
		return nil, err
	}

	outEv := unix.EpollEvent{
		Events: unix.EPOLLOUT | unix.EPOLLERR | unix.EPOLLHUP,
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

	return &tun{
		fd:    dup,
		epIn:  epIn,
		epOut: epOut,
		io:    iolifecycle.New(),
	}, nil
}

// Read is NOT safe to call concurrently with another Read on the same instance.
func (w *tun) Read(p []byte) (int, error) {
	if !w.io.TryAcquire() {
		return 0, io.ErrClosedPipe
	}
	defer w.io.Release()

	for {
		if w.io.Closing() {
			return 0, io.ErrClosedPipe
		}
		n, err := unix.Read(w.fd, p)
		if err == nil {
			if n == 0 {
				return 0, io.EOF
			}
			return n, nil
		}
		switch {
		case errors.Is(err, unix.EINTR):
			continue
		case errors.Is(err, unix.ENXIO) || errors.Is(err, unix.ENODEV):
			// Device went away (interface down/removed) – normalize to EOF.
			return 0, io.EOF
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

// Write is NOT safe to call concurrently with another Write on the same instance.
func (w *tun) Write(p []byte) (int, error) {
	if !w.io.TryAcquire() {
		return 0, io.ErrClosedPipe
	}
	defer w.io.Release()

	if len(p) == 0 {
		return 0, nil
	}
	total := 0
	for total < len(p) {
		if w.io.Closing() {
			return total, io.ErrClosedPipe
		}
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
		case errors.Is(err, unix.ENXIO) || errors.Is(err, unix.ENODEV):
			// Device went away – normalize to EOF to signal permanent link-down.
			return total, io.EOF
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

// Close marks the wrapper closed, drains active I/O, and closes all descriptors.
// Active epoll waits observe the closing state after at most epollWaitTimeoutMillis.
// It is safe to call multiple times.
func (w *tun) Close() error {
	w.closeOnce.Do(func() {
		w.io.Drain(nil)

		if err := unix.Close(w.epIn); err != nil {
			w.closeErr = err
		}
		if err := unix.Close(w.epOut); err != nil && w.closeErr == nil {
			w.closeErr = err
		}
		if err := unix.Close(w.fd); err != nil && w.closeErr == nil {
			w.closeErr = err
		}
	})
	return w.closeErr
}

func (w *tun) waitRead() error {
	var evs [1]unix.EpollEvent
	for {
		n, err := unix.EpollWait(w.epIn, evs[:], epollWaitTimeoutMillis)
		if errors.Is(err, unix.EINTR) {
			if w.io.Closing() {
				return io.ErrClosedPipe
			}
			continue
		}
		if err != nil {
			if errors.Is(err, unix.EBADF) || w.io.Closing() {
				return io.ErrClosedPipe
			}
			return err
		}
		if w.io.Closing() {
			return io.ErrClosedPipe
		}
		if n == 0 {
			continue
		}
		ev := evs[0].Events
		if (ev & (unix.EPOLLERR | unix.EPOLLHUP)) != 0 {
			return io.EOF
		}
		if (ev & unix.EPOLLIN) != 0 {
			return nil
		}
	}
}

func (w *tun) waitWrite() error {
	var evs [1]unix.EpollEvent
	for {
		n, err := unix.EpollWait(w.epOut, evs[:], epollWaitTimeoutMillis)
		if errors.Is(err, unix.EINTR) {
			if w.io.Closing() {
				return io.ErrClosedPipe
			}
			continue
		}
		if err != nil {
			if errors.Is(err, unix.EBADF) || w.io.Closing() {
				return io.ErrClosedPipe
			}
			return err
		}
		if w.io.Closing() {
			return io.ErrClosedPipe
		}
		if n == 0 {
			continue
		}
		ev := evs[0].Events
		if (ev & (unix.EPOLLERR | unix.EPOLLHUP)) != 0 {
			return io.EOF
		}
		if (ev & unix.EPOLLOUT) != 0 {
			return nil
		}
	}
}
