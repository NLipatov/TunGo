package tcp_chacha20

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"golang.org/x/crypto/chacha20poly1305"
	"io"
	"testing"
)

/* ─── mocks ──────────────────────────────────────────────────────────────── */

type TransportHandlerTestMockCrypt struct {
	decryptOutput []byte
	decryptErr    error
}

func (m *TransportHandlerTestMockCrypt) Encrypt(b []byte) ([]byte, error) { return b, nil }
func (m *TransportHandlerTestMockCrypt) Decrypt(_ []byte) ([]byte, error) {
	return m.decryptOutput, m.decryptErr
}

type TransportHandlerTestWriteCounter struct{ n int }

func (w *TransportHandlerTestWriteCounter) Write(p []byte) (int, error) { w.n++; return len(p), nil }

type TransportHandlerTestErrWriter struct{ err error }

func (e *TransportHandlerTestErrWriter) Write([]byte) (int, error) { return 0, e.err }

/* ─── tests ──────────────────────────────────────────────────────────────── */

func TestTransportHandler_ContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	h := NewTransportHandler(ctx, bytes.NewReader(nil), io.Discard, &TransportHandlerTestMockCrypt{})
	if err := h.HandleTransport(); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestTransportHandler_PrefixReadError(t *testing.T) {
	r := bytes.NewReader([]byte{1, 2}) // <4 → ErrUnexpectedEOF
	h := NewTransportHandler(context.Background(), r, io.Discard, &TransportHandlerTestMockCrypt{})
	if err := h.HandleTransport(); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("want ErrUnexpectedEOF, got %v", err)
	}
}

func TestTransportHandler_InvalidLength(t *testing.T) {
	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.BigEndian, uint32(3)) // invalid length
	h := NewTransportHandler(context.Background(), buf, io.Discard, &TransportHandlerTestMockCrypt{})
	if err := h.HandleTransport(); !errors.Is(err, io.EOF) {
		t.Fatalf("want EOF after invalid length, got %v", err)
	}
}

func TestTransportHandler_ReadPayloadError(t *testing.T) {
	buf := new(bytes.Buffer)

	// Declare a valid ciphertext length but provide fewer bytes than declared.
	total := chacha20poly1305.Overhead + 10 // declared
	_ = binary.Write(buf, binary.BigEndian, uint32(total))
	// Provide only (Overhead + 5): short by 5 bytes
	buf.Write(make([]byte, chacha20poly1305.Overhead+5))

	h := NewTransportHandler(context.Background(), buf, io.Discard, &TransportHandlerTestMockCrypt{})
	if err := h.HandleTransport(); !errors.Is(err, io.EOF) {
		t.Fatalf("want EOF after short read, got %v", err)
	}
}

func TestTransportHandler_DecryptError(t *testing.T) {
	payload := []byte{9, 9, 9, 9}
	decErr := errors.New("decrypt fail")
	crypt := &TransportHandlerTestMockCrypt{decryptErr: decErr}

	total := chacha20poly1305.Overhead + len(payload)
	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.BigEndian, uint32(total))
	buf.Write(make([]byte, total))

	h := NewTransportHandler(context.Background(), buf, io.Discard, crypt)
	if err := h.HandleTransport(); !errors.Is(err, decErr) {
		t.Fatalf("want decrypt error, got %v", err)
	}
}

func TestTransportHandler_WriteError(t *testing.T) {
	payload := []byte{7, 7, 7, 7}
	wErr := errors.New("write fail")
	w := &TransportHandlerTestErrWriter{err: wErr}
	crypt := &TransportHandlerTestMockCrypt{decryptOutput: payload}

	ciphertextLen := chacha20poly1305.Overhead + len(payload)

	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.BigEndian, uint32(ciphertextLen))
	randData := make([]byte, ciphertextLen)
	if _, err := rand.Read(randData); err != nil {
		t.Fatal(err)
	}
	buf.Write(randData)

	h := NewTransportHandler(context.Background(), buf, w, crypt)
	if err := h.HandleTransport(); !errors.Is(err, wErr) {
		t.Fatalf("want write error, got %v", err)
	}
}

func TestTransportHandler_SuccessEOF(t *testing.T) {
	payload := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	ciphertextLen := chacha20poly1305.Overhead + len(payload)

	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.BigEndian, uint32(ciphertextLen))
	randData := make([]byte, ciphertextLen)
	_, rErr := rand.Read(randData)
	if rErr != nil {
		t.Fatal(rErr)
	}
	buf.Write(randData)

	w := &TransportHandlerTestWriteCounter{}
	crypt := &TransportHandlerTestMockCrypt{decryptOutput: payload}

	h := NewTransportHandler(context.Background(), buf, w, crypt)
	if err := h.HandleTransport(); !errors.Is(err, io.EOF) {
		t.Fatalf("want io.EOF at end, got %v", err)
	}
	if w.n != 1 {
		t.Fatalf("want 1 write, got %d", w.n)
	}
}
