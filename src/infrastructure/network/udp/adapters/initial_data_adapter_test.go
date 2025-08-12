package adapters

import (
	"errors"
	"io"
	"testing"
	"tungo/application"
)

// InitialDataAdapter implements application.ConnectionAdapter
// and returns provided initialData on the first Read calls.
// Afterwards it proxies to the underlying adapter.

// mockInitialDataAdapter is a mock for testing InitialDataAdapter behavior
// and implements application.ConnectionAdapter.
type mockInitialDataAdapter struct {
	readBuf   []byte
	writeBuf  []byte
	closed    bool
	readCalls int
}

func (m *mockInitialDataAdapter) Read(p []byte) (int, error) {
	m.readCalls++
	if m.readCalls > 1 && len(m.readBuf) == 0 {
		return 0, io.EOF
	}
	n := copy(p, m.readBuf)
	m.readBuf = m.readBuf[n:]
	return n, nil
}

func (m *mockInitialDataAdapter) Write(p []byte) (int, error) {
	m.writeBuf = append(m.writeBuf, p...)
	return len(p), nil
}

func (m *mockInitialDataAdapter) Close() error {
	if m.closed {
		return errors.New("already closed")
	}
	m.closed = true
	return nil
}

func TestRead_PartialInitialData(t *testing.T) {
	under := &mockInitialDataAdapter{readBuf: []byte("xyz")}
	adapter := NewInitialDataAdapter(under, []byte("abcdefgh"))

	// buffer smaller than initialData
	buf := make([]byte, 3)
	n, err := adapter.Read(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := string(buf[:n]); got != "abc" {
		t.Errorf("expected \"abc\", got %q", got)
	}

	// remaining initialData should be returned next
	buf2 := make([]byte, 10)
	n2, err := adapter.Read(buf2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := string(buf2[:n2]); got != "defgh" {
		t.Errorf("expected \"defgh\", got %q", got)
	}
}

func TestRead_ExactAndThenProxy(t *testing.T) {
	under := &mockInitialDataAdapter{readBuf: []byte("payload")}
	adapter := NewInitialDataAdapter(under, []byte("INIT"))

	// exact buffer match for initialData
	buf := make([]byte, 4)
	_, err := adapter.Read(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(buf) != "INIT" {
		t.Errorf("expected INIT, got %q", buf)
	}

	// subsequent Read proxies to underlying adapter
	buf2 := make([]byte, 7)
	n2, err := adapter.Read(buf2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(buf2[:n2]) != "payload" {
		t.Errorf("expected payload, got %q", buf2[:n2])
	}
}

func TestRead_NoInitialData(t *testing.T) {
	under := &mockInitialDataAdapter{readBuf: []byte("hello")}
	adapter := NewInitialDataAdapter(under, nil)

	buf := make([]byte, 5)
	n, err := adapter.Read(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(buf[:n]) != "hello" {
		t.Errorf("expected \"hello\", got %q", buf[:n])
	}
}

func TestWriteAndClose(t *testing.T) {
	under := &mockInitialDataAdapter{}
	var conn application.ConnectionAdapter = NewInitialDataAdapter(under, nil)

	// Write should delegate to the underlying adapter
	n, err := conn.Write([]byte("abc"))
	if err != nil {
		t.Fatalf("write error: %v", err)
	}
	if n != 3 || string(under.writeBuf) != "abc" {
		t.Errorf("under.writeBuf = %q, want \"abc\"", under.writeBuf)
	}

	// Close should delegate to the underlying adapter
	if err := conn.Close(); err != nil {
		t.Fatalf("close error: %v", err)
	}
	if !under.closed {
		t.Errorf("under.closed = false, want true")
	}
}
