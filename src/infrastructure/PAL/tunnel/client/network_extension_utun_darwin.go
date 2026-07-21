//go:build darwin

package client

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

const (
	utunHeaderLength = 4
	sysProtoControl  = 2
	optIfName        = 2
)

// fileDescriptorUTUN owns a duplicate of a NetworkExtension-provided UTUN fd.
// It uses nonblocking I/O plus a cancellation pipe so Close reliably wakes
// reads and writes without closing the descriptor retained by the provider.
type fileDescriptorUTUN struct {
	fd         int
	cancelRead int
	cancelSend int

	mu     sync.Mutex
	closed bool
	active sync.WaitGroup
}

func newFileDescriptorUTUN(sourceFD int) (*fileDescriptorUTUN, error) {
	duplicate, err := unix.Dup(sourceFD)
	if err != nil {
		return nil, fmt.Errorf("duplicate UTUN file descriptor: %w", err)
	}
	if err := unix.SetNonblock(duplicate, true); err != nil {
		_ = unix.Close(duplicate)
		return nil, fmt.Errorf("configure UTUN file descriptor: %w", err)
	}

	var cancel [2]int
	if err := unix.Pipe(cancel[:]); err != nil {
		_ = unix.Close(duplicate)
		return nil, fmt.Errorf("create UTUN cancellation pipe: %w", err)
	}
	if err := unix.SetNonblock(cancel[1], true); err != nil {
		_ = unix.Close(cancel[0])
		_ = unix.Close(cancel[1])
		_ = unix.Close(duplicate)
		return nil, fmt.Errorf("configure UTUN cancellation pipe: %w", err)
	}
	return &fileDescriptorUTUN{fd: duplicate, cancelRead: cancel[0], cancelSend: cancel[1]}, nil
}

func (u *fileDescriptorUTUN) Read(frags [][]byte, sizes []int, offset int) (int, error) {
	if err := validateVectorRead(frags, sizes, offset); err != nil {
		return 0, err
	}
	if !u.beginOperation() {
		return 0, os.ErrClosed
	}
	defer u.active.Done()

	for {
		n, err := unix.Readv(u.fd, [][]byte{frags[0][:utunHeaderLength], frags[1]})
		switch {
		case err == nil:
			if n < utunHeaderLength {
				return 0, errors.New("short read (no UTUN header)")
			}
			sizes[0] = n - utunHeaderLength
			return 2, nil
		case errors.Is(err, unix.EINTR):
			continue
		case errors.Is(err, unix.EAGAIN):
			if err := u.wait(unix.POLLIN); err != nil {
				return 0, err
			}
		default:
			if u.isClosed() {
				return 0, os.ErrClosed
			}
			return 0, err
		}
	}
}

func (u *fileDescriptorUTUN) Write(frags [][]byte, offset int) (int, error) {
	if err := validateVectorWrite(frags, offset); err != nil {
		return 0, err
	}
	if !u.beginOperation() {
		return 0, os.ErrClosed
	}
	defer u.active.Done()

	for {
		n, err := unix.Writev(u.fd, [][]byte{frags[0][:utunHeaderLength], frags[1]})
		switch {
		case err == nil:
			if n < utunHeaderLength {
				return 0, errors.New("short write (no UTUN header)")
			}
			return n - utunHeaderLength, nil
		case errors.Is(err, unix.EINTR):
			continue
		case errors.Is(err, unix.EAGAIN):
			if err := u.wait(unix.POLLOUT); err != nil {
				return 0, err
			}
		default:
			if u.isClosed() {
				return 0, os.ErrClosed
			}
			return 0, err
		}
	}
}

func (u *fileDescriptorUTUN) Close() error {
	u.mu.Lock()
	if u.closed {
		u.mu.Unlock()
		return nil
	}
	u.closed = true
	u.mu.Unlock()

	_, _ = unix.Write(u.cancelSend, []byte{1})
	u.active.Wait()

	var result error
	for _, fd := range []int{u.fd, u.cancelRead, u.cancelSend} {
		if err := unix.Close(fd); err != nil && result == nil {
			result = err
		}
	}
	return result
}

func (u *fileDescriptorUTUN) Name() (string, error) {
	return unix.GetsockoptString(u.fd, sysProtoControl, optIfName)
}

func (u *fileDescriptorUTUN) beginOperation() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.closed {
		return false
	}
	u.active.Add(1)
	return true
}

func (u *fileDescriptorUTUN) isClosed() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.closed
}

func (u *fileDescriptorUTUN) wait(event int16) error {
	pollFDs := []unix.PollFd{
		{Fd: int32(u.fd), Events: event},
		{Fd: int32(u.cancelRead), Events: unix.POLLIN},
	}
	for {
		_, err := unix.Poll(pollFDs, -1)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			if u.isClosed() {
				return os.ErrClosed
			}
			return err
		}
		if pollFDs[1].Revents != 0 || u.isClosed() {
			return os.ErrClosed
		}
		if pollFDs[0].Revents&(event|unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0 {
			return nil
		}
	}
}

func validateVectorRead(frags [][]byte, sizes []int, offset int) error {
	if len(sizes) == 0 {
		return errors.New("sizes required")
	}
	if offset != 0 {
		return errors.New("offset must be 0 in vectored mode")
	}
	return validateVectors(frags)
}

func validateVectorWrite(frags [][]byte, offset int) error {
	if offset != 0 {
		return errors.New("offset must be 0 in vectored mode")
	}
	return validateVectors(frags)
}

func validateVectors(frags [][]byte) error {
	if len(frags) < 2 {
		return errors.New("need two buffers: hdr and payload")
	}
	if len(frags[0]) < utunHeaderLength {
		return errors.New("hdr too small (<4)")
	}
	return nil
}
