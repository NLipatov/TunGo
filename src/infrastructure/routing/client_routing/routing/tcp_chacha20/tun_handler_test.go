package tcp_chacha20

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"testing"

	"golang.org/x/crypto/chacha20poly1305"
)

// thTestMockCrypt implements CryptographyService with fixed outputs
type thTestMockCrypt struct {
	encryptOutput []byte
	encryptErr    error
}

func (m *thTestMockCrypt) Encrypt(b []byte) ([]byte, error) { return m.encryptOutput, m.encryptErr }
func (m *thTestMockCrypt) Decrypt(b []byte) ([]byte, error) { return nil, nil }

// thTestFakeReader simulates TUN reader
type thTestFakeReader struct {
	payload []byte
	called  bool
	err     error
}

func (f *thTestFakeReader) Read(p []byte) (int, error) {
	if !f.called {
		total := len(f.payload) + 4 + chacha20poly1305.Overhead
		binary.BigEndian.PutUint32(p, uint32(len(f.payload)))
		copy(p[4:], f.payload)
		f.called = true
		return total, nil
	}
	if f.err != nil {
		return 0, f.err
	}
	return 0, io.EOF
}

// thTestErrWriter always errors
type thTestErrWriter struct{ err error }

func (e *thTestErrWriter) Write(p []byte) (int, error) { return 0, e.err }

// thTestWriteCounter counts writes
type thTestWriteCounter struct{ count int }

func (w *thTestWriteCounter) Write(p []byte) (int, error) { w.count++; return len(p), nil }

func TestHandleTun_ContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f := &thTestFakeReader{}
	w := &thTestErrWriter{err: errors.New("no write")}
	h := NewTunHandler(ctx, f, w, &thTestMockCrypt{})
	if err := h.HandleTun(); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestHandleTun_ReadError(t *testing.T) {
	rErr := errors.New("read fail")
	f := &thTestFakeReader{err: rErr}
	h := NewTunHandler(context.Background(), f, io.Discard, &thTestMockCrypt{})
	if err := h.HandleTun(); !errors.Is(err, rErr) {
		t.Fatalf("expected read error, got %v", err)
	}
}

func TestHandleTun_EncryptError(t *testing.T) {
	payload := []byte{1, 2, 3}
	f := &thTestFakeReader{payload: payload}
	encryptErr := errors.New("encrypt fail")
	crypt := &thTestMockCrypt{encryptOutput: nil, encryptErr: encryptErr}
	h := NewTunHandler(context.Background(), f, io.Discard, crypt)
	if err := h.HandleTun(); !errors.Is(err, encryptErr) {
		t.Fatalf("expected encrypt error, got %v", err)
	}
}

func TestHandleTun_SuccessEOF(t *testing.T) {
	payload := []byte{0xAA, 0xBB}
	f := &thTestFakeReader{payload: payload}
	w := &thTestWriteCounter{}
	crypt := &thTestMockCrypt{encryptOutput: payload, encryptErr: nil}
	h := NewTunHandler(context.Background(), f, w, crypt)
	err := h.HandleTun()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF, got %v", err)
	}
	if w.count != 1 {
		t.Fatalf("expected 1 write, got %d", w.count)
	}
}

func TestHandleTun_WriteError(t *testing.T) {
	payload := []byte{0x0F}
	f := &thTestFakeReader{payload: payload}
	wErr := errors.New("write fail")
	w := &thTestErrWriter{err: wErr}
	crypt := &thTestMockCrypt{encryptOutput: payload, encryptErr: nil}
	h := NewTunHandler(context.Background(), f, w, crypt)
	if err := h.HandleTun(); !errors.Is(err, wErr) {
		t.Fatalf("expected write error, got %v", err)
	}
}
