//go:build linux

package epoll

import (
	"errors"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// makeSocketpair returns two connected bidirectional fds.
// We keep them blocking initially; newTUN will dup and set O_NONBLOCK on its side.
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

func TestCloseMakesFutureOpsFail(t *testing.T) {
	left, rightFD := makeSocketpair(t)
	defer func(fd int) {
		_ = unix.Close(fd)
	}(rightFD)

	dev, err := newTUN(left)
	if err != nil {
		t.Fatalf("newTUN: %v", err)
	}
	w := dev.(*tun)

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	buf := make([]byte, 1)
	if _, err := w.Read(buf); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("Read after Close: got %v, want io.ErrClosedPipe", err)
	}
	if _, err := w.Write([]byte{1}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("Write after Close: got %v, want io.ErrClosedPipe", err)
	}
}

func TestReadBlocksUntilDataThenReturns(t *testing.T) {
	left, rightFD := makeSocketpair(t)
	defer func(fd int) {
		_ = unix.Close(fd)
	}(rightFD)

	dev, err := newTUN(left) // newTUN takes ownership and closes 'left'
	if err != nil {
		t.Fatalf("newTUN: %v", err)
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

	// Ensure it blocks for a bit
	select {
	case <-readDone:
		t.Fatal("Read returned before any data was written (should block)")
	case <-time.After(50 * time.Millisecond):
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

	dev, err := newTUN(left)
	if err != nil {
		t.Fatalf("newTUN: %v", err)
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

	time.Sleep(50 * time.Millisecond)

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

	dev, err := newTUN(left)
	if err != nil {
		t.Fatalf("newTUN: %v", err)
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

	dev, err := newTUN(left)
	if err != nil {
		t.Fatalf("newTUN: %v", err)
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

	dev, err := newTUN(left)
	if err != nil {
		t.Fatalf("newTUN: %v", err)
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
