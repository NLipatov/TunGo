package tcp_chacha20

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"testing"

	"golang.org/x/crypto/chacha20poly1305"
)

// tunHandlerTestMockCrypt implements CryptographyService with fixed encryption outputs
type tunHandlerTestMockCrypt struct {
	encryptOutput []byte
	encryptErr    error
}

func (m *tunHandlerTestMockCrypt) Encrypt(b []byte) ([]byte, error) {
	return m.encryptOutput, m.encryptErr
}

func (m *tunHandlerTestMockCrypt) Decrypt(b []byte) ([]byte, error) {
	return nil, nil
}

// tunHandlerTestFakeReader simulates a TUN device reader
// first Read returns framed payload, then EOF or provided err
type tunHandlerTestFakeReader struct {
	payload []byte
	called  bool
	err     error
}

func (f *tunHandlerTestFakeReader) Read(p []byte) (int, error) {
	if !f.called {
		total := len(f.payload) + 4 + chacha20poly1305.Overhead
		// frame: 4-byte length + payload (rest filled by chacha20 reader wrapper)
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

// tunHandlerTestErrWriter always returns an error on Write
type tunHandlerTestErrWriter struct{ err error }

func (e *tunHandlerTestErrWriter) Write(p []byte) (int, error) {
	return 0, e.err
}

// writeCounter counts Write calls
type writeCounter struct {
	count int
}

func (w *writeCounter) Write(p []byte) (int, error) {
	w.count++
	return len(p), nil
}

func TestHandleTun_ContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f := &tunHandlerTestFakeReader{}
	w := &tunHandlerTestErrWriter{err: errors.New("no write")}
	h := NewTunHandler(ctx, f, w, &tunHandlerTestMockCrypt{encryptOutput: nil, encryptErr: nil})
	if err := h.HandleTun(); err != nil {
		t.Fatalf("expected nil on context done, got %v", err)
	}
}

func TestHandleTun_ReadError(t *testing.T) {
	rErr := errors.New("read fail")
	f := &tunHandlerTestFakeReader{err: rErr}
	h := NewTunHandler(context.Background(), f, io.Discard, &tunHandlerTestMockCrypt{encryptOutput: nil, encryptErr: nil})
	if err := h.HandleTun(); !errors.Is(err, rErr) {
		t.Fatalf("expected read error, got %v", err)
	}
}

func TestHandleTun_EncryptError(t *testing.T) {
	payload := []byte{0x01, 0x02, 0x03}
	f := &tunHandlerTestFakeReader{payload: payload}
	encryptErr := errors.New("encrypt fail")
	crypt := &tunHandlerTestMockCrypt{encryptOutput: nil, encryptErr: encryptErr}
	h := NewTunHandler(context.Background(), f, io.Discard, crypt)
	if err := h.HandleTun(); !errors.Is(err, encryptErr) {
		t.Fatalf("expected encrypt error, got %v", err)
	}
}

func TestHandleTun_SuccessEOF(t *testing.T) {
	payload := []byte{0xAA, 0xBB}
	f := &tunHandlerTestFakeReader{payload: payload}
	w := &writeCounter{}
	crypt := &tunHandlerTestMockCrypt{encryptOutput: append([]byte(nil), payload...), encryptErr: nil}
	h := NewTunHandler(context.Background(), f, w, crypt)
	err := h.HandleTun()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF after success, got %v", err)
	}
	if w.count != 1 {
		t.Fatalf("expected 1 write, got %d", w.count)
	}
}

func TestHandleTun_WriteError(t *testing.T) {
	payload := []byte{0x0F, 0x0E}
	f := &tunHandlerTestFakeReader{payload: payload}
	wErr := errors.New("write fail")
	w := &tunHandlerTestErrWriter{err: wErr}
	crypt := &tunHandlerTestMockCrypt{encryptOutput: append([]byte(nil), payload...), encryptErr: nil}
	h := NewTunHandler(context.Background(), f, w, crypt)
	if err := h.HandleTun(); !errors.Is(err, wErr) {
		t.Fatalf("expected write error, got %v", err)
	}
}
