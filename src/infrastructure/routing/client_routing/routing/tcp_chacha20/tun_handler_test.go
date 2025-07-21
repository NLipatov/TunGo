package tcp_chacha20

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

// --- mocks ---

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
	copy(b, r.payload)
	return len(r.payload), nil
}

type TunHandlerTestWriter struct {
	buf *bytes.Buffer
	err error
}

func (w *TunHandlerTestWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	return w.buf.Write(p)
}

type alwaysErrReader struct{}

func (alwaysErrReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

// --- tests ---

func TestTunHandler_ContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	th := NewTunHandler(ctx, &bytes.Buffer{}, &bytes.Buffer{}, &TunHandlerTestMockCrypt{})
	if err := th.HandleTun(); err != nil {
		t.Fatalf("expect nil, got %v", err)
	}
}

func TestTunHandler_ReadError(t *testing.T) {
	bad := alwaysErrReader{}
	th := NewTunHandler(context.Background(), bad, &bytes.Buffer{}, &TunHandlerTestMockCrypt{})
	if err := th.HandleTun(); err == nil {
		t.Fatalf("expected read error, got nil")
	}
}

func TestTunHandler_EncryptError(t *testing.T) {
	payload := make([]byte, 10)
	r := &TunHandlerTestReader{payload: payload, max: 1}
	th := NewTunHandler(context.Background(), r, &bytes.Buffer{}, &TunHandlerTestMockCrypt{encryptErr: errors.New("enc fail")})
	err := th.HandleTun()
	if !errors.Is(err, errors.New("enc fail")) && (err == nil || err.Error() != "enc fail") {
		t.Fatalf("want encrypt error, got %v", err)
	}
}

func TestTunHandler_WriteError(t *testing.T) {
	payload := make([]byte, 10)
	r := &TunHandlerTestReader{payload: payload, max: 1}
	w := &TunHandlerTestWriter{buf: &bytes.Buffer{}, err: errors.New("write fail")}
	th := NewTunHandler(context.Background(), r, w, &TunHandlerTestMockCrypt{})
	err := th.HandleTun()
	if !errors.Is(err, w.err) && (err == nil || err.Error() != "write fail") {
		t.Fatalf("want write error, got %v", err)
	}
}

func TestTunHandler_SuccessfulFlow(t *testing.T) {
	payload := []byte("testdata")
	r := &TunHandlerTestReader{payload: payload, max: 1}
	buf := &bytes.Buffer{}
	w := &TunHandlerTestWriter{buf: buf}
	th := NewTunHandler(context.Background(), r, w, &TunHandlerTestMockCrypt{})
	err := th.HandleTun()
	if err != io.EOF && err != nil {
		t.Fatalf("expect EOF or nil, got %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("no data written")
	}
}
