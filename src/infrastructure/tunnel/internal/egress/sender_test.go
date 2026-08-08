package egress

import (
	"bytes"
	"errors"
	"net/netip"
	"sync"
	"testing"
)

type mockCrypto struct {
	encryptErr error
}

func (m *mockCrypto) Encrypt(data []byte) ([]byte, error) {
	if m.encryptErr != nil {
		return nil, m.encryptErr
	}
	return append([]byte(nil), data...), nil
}

func (m *mockCrypto) Decrypt(data []byte) ([]byte, error) { return data, nil }

type mockWriter struct {
	mu      sync.Mutex
	packets [][]byte
	err     error
}

func (w *mockWriter) Write(packet []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err != nil {
		return 0, w.err
	}
	w.packets = append(w.packets, append([]byte(nil), packet...))
	return len(packet), nil
}

type mockWriteCloser struct {
	mockWriter
	closed bool
}

func (w *mockWriteCloser) Close() error {
	w.closed = true
	return nil
}

type mockAddrPortWriter struct {
	mockWriter
	addr netip.AddrPort
}

func (w *mockAddrPortWriter) SetAddrPort(addr netip.AddrPort) {
	w.addr = addr
}

func TestSenderSend(t *testing.T) {
	writer := &mockWriter{}
	sender := New(writer, &mockCrypto{})

	data := []byte("hello")
	if err := sender.Send(data); err != nil {
		t.Fatal(err)
	}
	if len(writer.packets) != 1 || !bytes.Equal(writer.packets[0], data) {
		t.Fatalf("packets = %v", writer.packets)
	}
}

func TestSenderReturnsEncryptError(t *testing.T) {
	want := errors.New("encrypt failed")
	writer := &mockWriter{}
	if err := New(writer, &mockCrypto{encryptErr: want}).Send(nil); !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
	if len(writer.packets) != 0 {
		t.Fatal("writer called after encryption failure")
	}
}

func TestSenderReturnsWriteError(t *testing.T) {
	want := errors.New("write failed")
	err := New(&mockWriter{err: want}, &mockCrypto{}).Send(nil)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func TestSenderClose(t *testing.T) {
	writer := &mockWriteCloser{}
	if err := New(writer, &mockCrypto{}).Close(); err != nil {
		t.Fatal(err)
	}
	if !writer.closed {
		t.Fatal("writer was not closed")
	}
	if err := New(&mockWriter{}, &mockCrypto{}).Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSenderSetAddrPort(t *testing.T) {
	writer := &mockAddrPortWriter{}
	want := netip.MustParseAddrPort("203.0.113.7:51820")
	sender := New(writer, &mockCrypto{})
	sender.SetAddrPort(want)
	if writer.addr != want {
		t.Fatalf("address = %v, want %v", writer.addr, want)
	}
	sender = New(&mockWriter{}, &mockCrypto{})
	sender.SetAddrPort(want)
}

func TestSenderSerializesConcurrentSends(t *testing.T) {
	writer := &mockWriter{}
	sender := New(writer, &mockCrypto{})

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = sender.Send([]byte{byte(i)})
		}()
	}
	wg.Wait()

	writer.mu.Lock()
	defer writer.mu.Unlock()
	if len(writer.packets) != 100 {
		t.Fatalf("writes = %d, want 100", len(writer.packets))
	}
}
