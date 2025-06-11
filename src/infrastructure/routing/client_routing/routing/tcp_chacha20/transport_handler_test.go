package tcp_chacha20

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

/* ─── mocks ──────────────────────────────────────────────────────────────── */

type TransportHandlerTestMockCrypt struct {
	decryptOutput []byte
	decryptErr    error
}

func (m *TransportHandlerTestMockCrypt) Encrypt(b []byte) ([]byte, error) { return b, nil }
func (m *TransportHandlerTestMockCrypt) Decrypt(b []byte) ([]byte, error) {
	return m.decryptOutput, m.decryptErr
}

type TransportHandlerTestWriteCounter struct{ n int }

func (w *TransportHandlerTestWriteCounter) Write(p []byte) (int, error) { w.n++; return len(p), nil }

type TransportHandlerTestErrWriter struct{ err error }

func (e *TransportHandlerTestErrWriter) Write([]byte) (int, error) { return 0, e.err }

func TransportHandlerTestBuildPacket(payload []byte) *bytes.Buffer {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, uint32(len(payload)))
	buf.Write(payload)
	return buf
}

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
	binary.Write(buf, binary.BigEndian, uint32(3)) // invalid length
	h := NewTransportHandler(context.Background(), buf, io.Discard, &TransportHandlerTestMockCrypt{})
	if err := h.HandleTransport(); !errors.Is(err, io.EOF) {
		t.Fatalf("want EOF after invalid length, got %v", err)
	}
}

func TestTransportHandler_ReadPayloadError(t *testing.T) {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, uint32(6))
	buf.Write([]byte{1, 2, 3}) // short payload
	h := NewTransportHandler(context.Background(), buf, io.Discard, &TransportHandlerTestMockCrypt{})
	if err := h.HandleTransport(); !errors.Is(err, io.EOF) {
		t.Fatalf("want EOF after short read, got %v", err)
	}
}

func TestTransportHandler_DecryptError(t *testing.T) {
	payload := []byte{9, 9, 9, 9}
	decErr := errors.New("decrypt fail")
	crypt := &TransportHandlerTestMockCrypt{decryptErr: decErr}
	h := NewTransportHandler(context.Background(), TransportHandlerTestBuildPacket(payload), io.Discard, crypt)
	if err := h.HandleTransport(); !errors.Is(err, decErr) {
		t.Fatalf("want decrypt error, got %v", err)
	}
}

func TestTransportHandler_WriteError(t *testing.T) {
	payload := []byte{7, 7, 7, 7}
	wErr := errors.New("write fail")
	w := &TransportHandlerTestErrWriter{err: wErr}
	crypt := &TransportHandlerTestMockCrypt{decryptOutput: payload}
	h := NewTransportHandler(context.Background(), TransportHandlerTestBuildPacket(payload), w, crypt)
	if err := h.HandleTransport(); !errors.Is(err, wErr) {
		t.Fatalf("want write error, got %v", err)
	}
}

func TestTransportHandler_SuccessEOF(t *testing.T) {
	payload := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	w := &TransportHandlerTestWriteCounter{}
	crypt := &TransportHandlerTestMockCrypt{decryptOutput: payload}
	h := NewTransportHandler(context.Background(), TransportHandlerTestBuildPacket(payload), w, crypt)
	if err := h.HandleTransport(); !errors.Is(err, io.EOF) {
		t.Fatalf("want io.EOF at end, got %v", err)
	}
	if w.n != 1 {
		t.Fatalf("want 1 write, got %d", w.n)
	}
}
