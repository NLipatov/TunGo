package adapters

import (
	"errors"
	"testing"
	"time"
)

// noDeadlineTransport implements connection.Transport but NOT SetReadDeadline.
type noDeadlineTransport struct {
	readData []byte
}

func (t *noDeadlineTransport) Read(p []byte) (int, error) {
	n := copy(p, t.readData)
	return n, nil
}
func (t *noDeadlineTransport) Write(p []byte) (int, error) { return len(p), nil }
func (t *noDeadlineTransport) Close() error                { return nil }

// deadlineTransport implements connection.Transport AND SetReadDeadline.
type deadlineTransport struct {
	readData     []byte
	writeData    []byte
	deadlineHits int
	lastDeadline time.Time
	closeErr     error
	closed       bool
}

func (t *deadlineTransport) Read(p []byte) (int, error) {
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

func TestNewReadDeadlineTransport_NoDeadlineSupport_ReturnsSame(t *testing.T) {
	tr := &noDeadlineTransport{}
	wrapped := NewReadDeadlineTransport(tr, time.Second)
	if wrapped != tr {
		t.Fatal("expected same transport when SetReadDeadline is not supported")
	}
}

func TestNewReadDeadlineTransport_WithDeadlineSupport_Wraps(t *testing.T) {
	tr := &deadlineTransport{readData: []byte("hello")}
	wrapped := NewReadDeadlineTransport(tr, time.Second)
	if wrapped == tr {
		t.Fatal("expected a wrapped transport")
	}
}

func TestReadDeadlineTransport_Read_SetsDeadlineBeforeRead(t *testing.T) {
	tr := &deadlineTransport{readData: []byte("abc")}
	timeout := 5 * time.Second
	wrapped := NewReadDeadlineTransport(tr, timeout)

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
	// Deadline should be approximately now + timeout
	expectedMin := before.Add(timeout)
	if tr.lastDeadline.Before(expectedMin.Add(-time.Second)) {
		t.Fatalf("deadline too early: %v, expected near %v", tr.lastDeadline, expectedMin)
	}
}

func TestReadDeadlineTransport_MultipleReads_RefreshDeadline(t *testing.T) {
	tr := &deadlineTransport{readData: []byte("xy")}
	wrapped := NewReadDeadlineTransport(tr, time.Second)

	buf := make([]byte, 2)
	_, _ = wrapped.Read(buf)
	_, _ = wrapped.Read(buf)
	_, _ = wrapped.Read(buf)

	if tr.deadlineHits != 3 {
		t.Fatalf("expected 3 deadline sets, got %d", tr.deadlineHits)
	}
}

func TestReadDeadlineTransport_Write_PassesThrough(t *testing.T) {
	tr := &deadlineTransport{}
	wrapped := NewReadDeadlineTransport(tr, time.Second)

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

func TestReadDeadlineTransport_Close_PassesThrough(t *testing.T) {
	tr := &deadlineTransport{}
	wrapped := NewReadDeadlineTransport(tr, time.Second)

	if err := wrapped.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !tr.closed {
		t.Fatal("expected underlying transport to be closed")
	}
}

func TestReadDeadlineTransport_Close_PropagatesError(t *testing.T) {
	closeErr := errors.New("close failed")
	tr := &deadlineTransport{closeErr: closeErr}
	wrapped := NewReadDeadlineTransport(tr, time.Second)

	if err := wrapped.Close(); !errors.Is(err, closeErr) {
		t.Fatalf("expected %v, got %v", closeErr, err)
	}
}
