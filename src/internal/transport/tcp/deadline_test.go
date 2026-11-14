package tcp

import (
	"errors"
	"net"
	"os"
	"testing"
	"time"
)

type deadlineTransport struct {
	net.Conn
	readData      []byte
	writeData     []byte
	deadlineHits  int
	lastDeadline  time.Time
	readDeadline  time.Time
	writeDeadline time.Time
	closeErr      error
	closed        bool
}

func (t *deadlineTransport) Read(p []byte) (int, error) {
	t.readDeadline = t.lastDeadline
	n := copy(p, t.readData)
	return n, nil
}
func (t *deadlineTransport) Write(p []byte) (int, error) {
	t.writeData = append(t.writeData, p...)
	return len(p), nil
}
func (t *deadlineTransport) Close() error {
	t.closed = true
	return t.closeErr
}
func (t *deadlineTransport) SetReadDeadline(dl time.Time) error {
	t.deadlineHits++
	t.lastDeadline = dl
	return nil
}

func (t *deadlineTransport) SetWriteDeadline(dl time.Time) error {
	t.writeDeadline = dl
	return nil
}

func (t *deadlineTransport) SetDeadline(dl time.Time) error {
	_ = t.SetReadDeadline(dl)
	return t.SetWriteDeadline(dl)
}

func TestReadDeadlineConn_Read_SetsDeadlineBeforeRead(t *testing.T) {
	tr := &deadlineTransport{readData: []byte("abc")}
	timeout := 5 * time.Second
	wrapped := WithReadDeadline(tr, timeout)

	before := time.Now()
	buf := make([]byte, 3)
	n, err := wrapped.Read(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 3 || string(buf) != "abc" {
		t.Fatalf("unexpected read: n=%d buf=%q", n, string(buf))
	}
	if tr.deadlineHits != 1 {
		t.Fatalf("expected 1 deadline set, got %d", tr.deadlineHits)
	}
	if tr.readDeadline.Before(before.Add(timeout)) || tr.readDeadline.After(time.Now().Add(timeout)) {
		t.Fatalf("deadline was not set to now + timeout before Read: %v", tr.readDeadline)
	}
}

func TestReadDeadlineConn_MultipleReads_RefreshDeadline(t *testing.T) {
	tr := &deadlineTransport{readData: []byte("xy")}
	wrapped := WithReadDeadline(tr, time.Second)

	buf := make([]byte, 2)
	for range 3 {
		before := time.Now()
		_, _ = wrapped.Read(buf)
		if tr.readDeadline.Before(before.Add(time.Second)) {
			t.Fatalf("read reused an old deadline: %v", tr.readDeadline)
		}
	}

	if tr.deadlineHits != 3 {
		t.Fatalf("expected 3 deadline sets, got %d", tr.deadlineHits)
	}
}

func TestReadDeadlineConn_ReadPreservesWriteDeadline(t *testing.T) {
	tr := &deadlineTransport{readData: []byte("x")}
	wrapped := WithReadDeadline(tr, time.Second)
	deadline := time.Now().Add(-time.Second)
	if err := wrapped.SetWriteDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	if _, err := wrapped.Read(make([]byte, 1)); err != nil {
		t.Fatal(err)
	}
	if tr.writeDeadline != deadline {
		t.Fatalf("Read changed write deadline to %v, want %v", tr.writeDeadline, deadline)
	}
}

func TestReadDeadlineConn_IdleReadTimesOut(t *testing.T) {
	conn, peer := net.Pipe()
	defer func() { _ = conn.Close() }()
	defer func() { _ = peer.Close() }()
	wrapped := WithReadDeadline(conn, 50*time.Millisecond)
	_, err := wrapped.Read(make([]byte, 1))
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("idle Read error = %v, want deadline exceeded", err)
	}
}

func TestReadDeadlineConn_Write_PassesThrough(t *testing.T) {
	tr := &deadlineTransport{}
	wrapped := WithReadDeadline(tr, time.Second)

	data := []byte("payload")
	n, err := wrapped.Write(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(data) {
		t.Fatalf("expected %d bytes written, got %d", len(data), n)
	}
	if string(tr.writeData) != "payload" {
		t.Fatalf("write not forwarded: got %q", string(tr.writeData))
	}
}

func TestReadDeadlineConn_Close_PassesThrough(t *testing.T) {
	tr := &deadlineTransport{}
	wrapped := WithReadDeadline(tr, time.Second)

	if err := wrapped.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !tr.closed {
		t.Fatal("expected underlying transport to be closed")
	}
}

func TestReadDeadlineConn_Close_PropagatesError(t *testing.T) {
	closeErr := errors.New("close failed")
	tr := &deadlineTransport{closeErr: closeErr}
	wrapped := WithReadDeadline(tr, time.Second)

	if err := wrapped.Close(); !errors.Is(err, closeErr) {
		t.Fatalf("expected %v, got %v", closeErr, err)
	}
}
