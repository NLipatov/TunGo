//go:build linux

package epoll

import (
	"errors"
	"io"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// makeSocketpair returns two connected bidirectional fds.
// We keep them blocking initially; New will dup and set O_NONBLOCK on its side.
func makeSocketpair(t *testing.T) (left *os.File, rightFD int) {
	t.Helper()
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("socketpair: %v", err)
	}
	left = os.NewFile(uintptr(fds[0]), "left")
	rightFD = fds[1]
	return
}

func reuseDescriptor(t *testing.T, fd int) int {
	t.Helper()

	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("create replacement socket pair: %v", err)
	}
	target, peer := fds[0], fds[1]
	if peer == fd {
		target, peer = peer, target
	}
	if target != fd {
		if err := unix.Dup2(target, fd); err != nil {
			_ = unix.Close(fds[0])
			_ = unix.Close(fds[1])
			t.Fatalf("reuse descriptor %d: %v", fd, err)
		}
	}
	t.Cleanup(func() {
		_ = unix.Close(fd)
		if target != fd {
			_ = unix.Close(target)
		}
		_ = unix.Close(peer)
	})
	return peer
}

func newTestTun(t *testing.T) *tun {
	t.Helper()
	left, rightFD := makeSocketpair(t)
	t.Cleanup(func() { _ = unix.Close(rightFD) })

	dev, err := New(left)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = dev.Close() })
	return dev.(*tun)
}

func TestCloseMakesFutureOpsFail(t *testing.T) {
	left, rightFD := makeSocketpair(t)
	defer func(fd int) {
		_ = unix.Close(fd)
	}(rightFD)

	dev, err := New(left)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	w := dev.(*tun)
	fd := w.fd

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	peer := reuseDescriptor(t, fd)
	if _, err := unix.Write(peer, []byte{1}); err != nil {
		t.Fatalf("write replacement socket: %v", err)
	}

	buf := make([]byte, 1)
	if _, err := w.Read(buf); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("Read after Close: got %v, want io.ErrClosedPipe", err)
	}
	n, _, err := unix.Recvfrom(fd, buf, unix.MSG_DONTWAIT)
	if err != nil {
		t.Fatalf("replacement socket data was consumed: %v", err)
	}
	if n != 1 || buf[0] != 1 {
		t.Fatalf("replacement socket data = %v, want [1]", buf[:n])
	}
	if _, err := w.Write([]byte{1}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("Write after Close: got %v, want io.ErrClosedPipe", err)
	}
	if _, _, err := unix.Recvfrom(peer, buf, unix.MSG_DONTWAIT); !errors.Is(err, unix.EAGAIN) {
		t.Fatalf("replacement socket received data, err = %v", err)
	}
}

func TestCloseDrainsActiveIOBeforeClosingDescriptors(t *testing.T) {
	left, rightFD := makeSocketpair(t)
	defer func() { _ = unix.Close(rightFD) }()

	dev, err := New(left)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	w := dev.(*tun)

	if !w.io.TryAcquire() {
		t.Fatal("failed to acquire I/O on an open tunnel")
	}
	releasePending := true
	defer func() {
		if releasePending {
			w.io.Release()
		}
	}()

	closeDone := make(chan error, 1)
	go func() {
		closeDone <- w.Close()
	}()

	deadline := time.Now().Add(time.Second)
	for !w.io.Closing() {
		if time.Now().After(deadline) {
			t.Fatal("Close did not start")
		}
		runtime.Gosched()
	}

	select {
	case err := <-closeDone:
		t.Fatalf("Close returned with active I/O: %v", err)
	default:
	}
	for _, fd := range []int{w.fd, w.epIn, w.epOut} {
		if _, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0); err != nil {
			t.Fatalf("descriptor %d was closed with active I/O: %v", fd, err)
		}
	}

	w.io.Release()
	releasePending = false
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not return after I/O drained")
	}
}

func TestCloseCachesDescriptorErrors(t *testing.T) {
	tests := []struct {
		name string
		fd   func(*tun) int
	}{
		{name: "read epoll", fd: func(w *tun) int { return w.epIn }},
		{name: "write epoll", fd: func(w *tun) int { return w.epOut }},
		{name: "TUN", fd: func(w *tun) int { return w.fd }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newTestTun(t)
			if err := unix.Close(tt.fd(w)); err != nil {
				t.Fatalf("close descriptor: %v", err)
			}

			firstErr := w.Close()
			if !errors.Is(firstErr, unix.EBADF) {
				t.Fatalf("first Close error = %v, want EBADF", firstErr)
			}
			if secondErr := w.Close(); !errors.Is(secondErr, firstErr) {
				t.Fatalf("second Close error = %v, want cached %v", secondErr, firstErr)
			}
		})
	}
}

func TestWaitReadReturnsClosedPipeForClosedEpoll(t *testing.T) {
	w := newTestTun(t)
	if err := unix.Close(w.epIn); err != nil {
		t.Fatalf("close read epoll: %v", err)
	}
	if err := w.waitRead(); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("waitRead error = %v, want io.ErrClosedPipe", err)
	}
}

func TestWaitWriteReturnsClosedPipeForClosedEpoll(t *testing.T) {
	w := newTestTun(t)
	if err := unix.Close(w.epOut); err != nil {
		t.Fatalf("close write epoll: %v", err)
	}
	if err := w.waitWrite(); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("waitWrite error = %v, want io.ErrClosedPipe", err)
	}
}

func TestWaitReadReturnsUnexpectedEpollError(t *testing.T) {
	w := newTestTun(t)
	epIn := w.epIn
	w.epIn = w.fd
	defer func() { w.epIn = epIn }()

	if err := w.waitRead(); !errors.Is(err, unix.EINVAL) {
		t.Fatalf("waitRead error = %v, want EINVAL", err)
	}
}

func TestWaitWriteReturnsUnexpectedEpollError(t *testing.T) {
	w := newTestTun(t)
	epOut := w.epOut
	w.epOut = w.fd
	defer func() { w.epOut = epOut }()

	if err := w.waitWrite(); !errors.Is(err, unix.EINVAL) {
		t.Fatalf("waitWrite error = %v, want EINVAL", err)
	}
}

func TestCloseUnblocksRead(t *testing.T) {
	left, rightFD := makeSocketpair(t)
	defer func(fd int) {
		_ = unix.Close(fd)
	}(rightFD)

	dev, err := New(left)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	readDone := make(chan error, 1)
	go func() {
		_, err := dev.Read(make([]byte, 1))
		readDone <- err
	}()

	time.Sleep(50 * time.Millisecond)
	if err := dev.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case err := <-readDone:
		if !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("Read after Close: got %v, want io.ErrClosedPipe", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Read did not unblock after Close")
	}
}

func TestCloseUnblocksWrite(t *testing.T) {
	left, rightFD := makeSocketpair(t)
	defer func(fd int) {
		_ = unix.Close(fd)
	}(rightFD)

	if err := unix.SetsockoptInt(int(left.Fd()), unix.SOL_SOCKET, unix.SO_SNDBUF, 4096); err != nil {
		t.Fatalf("set send buffer size: %v", err)
	}

	dev, err := New(left)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	writeDone := make(chan error, 1)
	go func() {
		_, err := dev.Write(make([]byte, 1<<20))
		writeDone <- err
	}()

	time.Sleep(50 * time.Millisecond)
	if err := dev.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case err := <-writeDone:
		if !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("Write after Close: got %v, want io.ErrClosedPipe", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Write did not unblock after Close")
	}
}

func TestReadBlocksUntilDataThenReturns(t *testing.T) {
	left, rightFD := makeSocketpair(t)
	defer func(fd int) {
		_ = unix.Close(fd)
	}(rightFD)

	dev, err := New(left) // New takes ownership and closes 'left'
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = dev.Close() })

	w := dev.(*tun)

	// Start a blocking read
	readDone := make(chan struct{})
	var n int
	var rerr error
	buf := make([]byte, 32)
	go func() {
		n, rerr = w.Read(buf)
		close(readDone)
	}()

	// Ensure it stays blocked across an epoll timeout.
	select {
	case <-readDone:
		t.Fatal("Read returned before any data was written (should block)")
	case <-time.After(time.Duration(epollWaitTimeoutMillis+50) * time.Millisecond):
	}

	// Write data from peer
	msg := []byte("hello-epoll")
	if _, err := unix.Write(rightFD, msg); err != nil {
		t.Fatalf("peer write: %v", err)
	}

	// Now it should finish quickly
	select {
	case <-readDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Read did not return after peer wrote data")
	}

	if rerr != nil {
		t.Fatalf("Read error: %v", rerr)
	}
	if n != len(msg) {
		t.Fatalf("Read bytes=%d want=%d", n, len(msg))
	}
	if string(buf[:n]) != string(msg) {
		t.Fatalf("payload mismatch: got %q want %q", buf[:n], msg)
	}
}

func TestWriteBackpressureWaitsAndCompletes(t *testing.T) {
	left, rightFD := makeSocketpair(t)
	defer func(fd int) {
		_ = unix.Close(fd)
	}(rightFD)

	_ = unix.SetsockoptInt(rightFD, unix.SOL_SOCKET, unix.SO_RCVBUF, 4096)

	dev, err := New(left)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = dev.Close() })

	w := dev.(*tun)

	payload := make([]byte, 1<<20) // 1 MiB
	for i := range payload {
		payload[i] = byte(i)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	writeErrCh := make(chan error, 1)
	go func() {
		defer wg.Done()
		_, err := w.Write(payload)
		writeErrCh <- err
	}()

	select {
	case err := <-writeErrCh:
		t.Fatalf("Write returned while the peer was not reading: %v", err)
	case <-time.After(time.Duration(epollWaitTimeoutMillis+50) * time.Millisecond):
	}

	total := 0
	tmp := make([]byte, 8192)
	for total < len(payload) {
		n, err := unix.Read(rightFD, tmp)
		if err != nil {
			t.Fatalf("peer read: %v", err)
		}
		total += n
		time.Sleep(1 * time.Millisecond)
	}

	wg.Wait()

	if err := <-writeErrCh; err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
}

func TestZeroLengthWrite(t *testing.T) {
	left, rightFD := makeSocketpair(t)
	defer func(fd int) {
		_ = unix.Close(fd)
	}(rightFD)

	dev, err := New(left)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = dev.Close() })

	w := dev.(*tun)

	n, err := w.Write(nil)
	if err != nil || n != 0 {
		t.Fatalf("Write(nil) = (%d, %v); want (0, nil)", n, err)
	}
}

func TestEOFOnPeerClose(t *testing.T) {
	left, rightFD := makeSocketpair(t)

	dev, err := New(left)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = dev.Close() })

	w := dev.(*tun)

	// Close peer completely -> our next Read should return EOF
	_ = unix.Close(rightFD)

	buf := make([]byte, 16)
	n, err := w.Read(buf)
	if n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("Read after peer close: (%d, %v); want (0, io.EOF)", n, err)
	}
}

func TestReadUnblocksOnPeerCloseWithEOF(t *testing.T) {
	left, rightFD := makeSocketpair(t)

	dev, err := New(left)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = dev.Close() })
	w := dev.(*tun)

	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 16)
		_, err := w.Read(buf) // blocks until peer closes
		done <- err
	}()

	time.Sleep(50 * time.Millisecond) // let it park in epoll_wait
	_ = unix.Close(rightFD)           // external wake: EPOLLHUP -> EOF

	select {
	case err := <-done:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("Read after peer close: got %v, want io.EOF", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Read did not unblock after peer close")
	}
}
