package tcp_chacha20

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"testing"
	"tungo/infrastructure/cryptography/chacha20"

	"golang.org/x/crypto/chacha20poly1305"
)

/* ─── mocks ──────────────────────────────────────────────────────────────── */

type TunHandlerTestMockCrypt struct{ encryptErr error }

func (m *TunHandlerTestMockCrypt) Encrypt(b []byte) ([]byte, error) { return b, m.encryptErr }
func (TunHandlerTestMockCrypt) Decrypt([]byte) ([]byte, error)      { return nil, nil }

type TunHandlerTestReader struct {
	payload []byte
	count   int
	max     int
}

func (r *TunHandlerTestReader) Read(b []byte) (int, error) {
	if r.count >= r.max {
		return 0, io.EOF
	}
	r.count++
	total := len(r.payload) + 4 + chacha20poly1305.Overhead
	binary.BigEndian.PutUint32(b, uint32(len(r.payload)))
	copy(b[4:], r.payload)
	return total, nil
}

type TunHandlerTestFakeEncoder struct{ failOnce bool }

func (e *TunHandlerTestFakeEncoder) Decode(data []byte, packet *chacha20.TCPPacket) error {
	return nil
}

func (e *TunHandlerTestFakeEncoder) Encode([]byte) error {
	if !e.failOnce {
		e.failOnce = true
		return errors.New("encode fail")
	}
	return nil
}

type TunHandlerTestOKEncoder struct{}

func (e TunHandlerTestOKEncoder) Decode(data []byte, packet *chacha20.TCPPacket) error {
	return nil
}

func (TunHandlerTestOKEncoder) Encode([]byte) error { return nil }

type TunHandlerTestWriteCounter struct{ n int }

func (w *TunHandlerTestWriteCounter) Write(p []byte) (int, error) { w.n++; return len(p), nil }

type TunHandlerTestErrWriter struct{ err error }

func (e *TunHandlerTestErrWriter) Write([]byte) (int, error) { return 0, e.err }

/* ─── tests ──────────────────────────────────────────────────────────────── */

func TestTunHandler_ContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	th := NewTunHandler(ctx,
		&TunHandlerTestFakeEncoder{},
		&TunHandlerTestReader{},
		io.Discard,
		&TunHandlerTestMockCrypt{},
	)
	if err := th.HandleTun(); err != nil {
		t.Fatalf("expect nil, got %v", err)
	}
}

func TestTunHandler_ReadError(t *testing.T) {
	bad := &struct{ io.Reader }{Reader: &bytes.Reader{}}
	th := NewTunHandler(context.Background(),
		&TunHandlerTestFakeEncoder{},
		bad,
		io.Discard,
		&TunHandlerTestMockCrypt{},
	)
	if err := th.HandleTun(); err == nil {
		t.Fatalf("expected read error, got nil")
	}
}

func TestTunHandler_EncryptError(t *testing.T) {
	r := &TunHandlerTestReader{payload: []byte{1, 2}, max: 1}
	encErr := errors.New("enc fail")
	th := NewTunHandler(context.Background(),
		&TunHandlerTestFakeEncoder{},
		r,
		io.Discard,
		&TunHandlerTestMockCrypt{encryptErr: encErr},
	)
	if err := th.HandleTun(); !errors.Is(err, encErr) {
		t.Fatalf("want encrypt error, got %v", err)
	}
}

func TestTunHandler_EncodeContinueAndSuccess(t *testing.T) {
	r := &TunHandlerTestReader{payload: []byte{0xAA}, max: 2}
	w := &TunHandlerTestWriteCounter{}
	enc := &TunHandlerTestFakeEncoder{}
	th := NewTunHandler(context.Background(), enc, r, w, &TunHandlerTestMockCrypt{})
	err := th.HandleTun()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("want EOF, got %v", err)
	}
	if w.n != 1 {
		t.Fatalf("expect 1 write, got %d", w.n)
	}
}

func TestTunHandler_WriteError(t *testing.T) {
	r := &TunHandlerTestReader{payload: []byte{1, 2, 3}, max: 1}

	wErr := errors.New("write fail")
	w := &TunHandlerTestErrWriter{err: wErr}

	enc := TunHandlerTestOKEncoder{}

	th := NewTunHandler(context.Background(), enc, r, w, &TunHandlerTestMockCrypt{})
	if err := th.HandleTun(); !errors.Is(err, wErr) {
		t.Fatalf("want write error, got %v", err)
	}
}
