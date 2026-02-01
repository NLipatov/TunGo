package connection

import (
	"bytes"
	"errors"
	"sync"
	"testing"
)

// egressMockCrypto implements Crypto for egress tests.
type egressMockCrypto struct {
	encErr error
}

func (m *egressMockCrypto) Encrypt(b []byte) ([]byte, error) {
	if m.encErr != nil {
		return nil, m.encErr
	}
	return append([]byte(nil), b...), nil
}

func (m *egressMockCrypto) Decrypt(b []byte) ([]byte, error) { return b, nil }

// egressMockWriter captures writes.
type egressMockWriter struct {
	mu   sync.Mutex
	data [][]byte
	err  error
}

func (w *egressMockWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err != nil {
		return 0, w.err
	}
	buf := make([]byte, len(p))
	copy(buf, p)
	w.data = append(w.data, buf)
	return len(p), nil
}

// egressMockWriteCloser adds Close() to egressMockWriter.
type egressMockWriteCloser struct {
	egressMockWriter
	closed bool
}

func (wc *egressMockWriteCloser) Close() error {
	wc.closed = true
	return nil
}

func TestNewDefaultEgress(t *testing.T) {
	e := NewDefaultEgress(&egressMockWriter{}, &egressMockCrypto{})
	if e == nil {
		t.Fatal("expected non-nil egress")
	}
}

func TestDefaultEgress_SendDataIP_Success(t *testing.T) {
	w := &egressMockWriter{}
	e := NewDefaultEgress(w, &egressMockCrypto{})

	data := []byte("hello")
	if err := e.SendDataIP(data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(w.data) != 1 || !bytes.Equal(w.data[0], data) {
		t.Fatalf("expected write of %v, got %v", data, w.data)
	}
}

func TestDefaultEgress_SendControl_Success(t *testing.T) {
	w := &egressMockWriter{}
	e := NewDefaultEgress(w, &egressMockCrypto{})

	data := []byte("ctrl")
	if err := e.SendControl(data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(w.data) != 1 || !bytes.Equal(w.data[0], data) {
		t.Fatalf("expected write of %v, got %v", data, w.data)
	}
}

func TestDefaultEgress_SendDataIP_EncryptError(t *testing.T) {
	encErr := errors.New("encrypt failed")
	w := &egressMockWriter{}
	e := NewDefaultEgress(w, &egressMockCrypto{encErr: encErr})

	if err := e.SendDataIP([]byte("data")); !errors.Is(err, encErr) {
		t.Fatalf("expected encrypt error, got %v", err)
	}
	if len(w.data) != 0 {
		t.Fatal("expected no writes on encrypt error")
	}
}

func TestDefaultEgress_SendControl_WriteError(t *testing.T) {
	writeErr := errors.New("write failed")
	w := &egressMockWriter{err: writeErr}
	e := NewDefaultEgress(w, &egressMockCrypto{})

	if err := e.SendControl([]byte("data")); !errors.Is(err, writeErr) {
		t.Fatalf("expected write error, got %v", err)
	}
}

func TestDefaultEgress_Close_WithCloser(t *testing.T) {
	wc := &egressMockWriteCloser{}
	e := NewDefaultEgress(wc, &egressMockCrypto{})

	if err := e.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !wc.closed {
		t.Fatal("expected writer to be closed")
	}
}

func TestDefaultEgress_Close_WithoutCloser(t *testing.T) {
	w := &egressMockWriter{}
	e := NewDefaultEgress(w, &egressMockCrypto{})

	if err := e.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDefaultEgress_ConcurrentSend(t *testing.T) {
	w := &egressMockWriter{}
	e := NewDefaultEgress(w, &egressMockCrypto{})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = e.SendDataIP([]byte{byte(n)})
		}(i)
	}
	wg.Wait()

	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.data) != 100 {
		t.Fatalf("expected 100 writes, got %d", len(w.data))
	}
}
