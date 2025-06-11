package tcp_chacha20

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

// transportHandlerTestMockCrypt allows injecting cryptography behavior
type transportHandlerTestMockCrypt struct {
	decryptFunc func([]byte) ([]byte, error)
}

func (m *transportHandlerTestMockCrypt) Encrypt(b []byte) ([]byte, error) {
	return b, nil
}

func (m *transportHandlerTestMockCrypt) Decrypt(b []byte) ([]byte, error) {
	return m.decryptFunc(b)
}

// errWriter always returns an error on Write
type errWriter struct{ err error }

func (e *errWriter) Write(_ []byte) (int, error) {
	return 0, e.err
}

func TestHandleTransport_SuccessEOF(t *testing.T) {
	// after successful packet, next read returns ErrUnexpectedEOF
	encrypted := []byte{1, 2, 3, 4}
	plaintext := []byte("OK")

	crypt := &transportHandlerTestMockCrypt{decryptFunc: func(b []byte) ([]byte, error) {
		if !bytes.Equal(b, encrypted) {
			t.Fatalf("unexpected data: %v", b)
		}
		return plaintext, nil
	}}

	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.BigEndian, uint32(len(encrypted)))
	buf.Write(encrypted)

	h := NewTransportHandler(context.Background(), buf, buf, crypt)
	err := h.HandleTransport()
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected ErrUnexpectedEOF, got %v", err)
	}
}

func TestHandleTransport_PrefixReadError(t *testing.T) {
	// reader shorter than prefix
	r := bytes.NewReader([]byte{0, 1})
	h := NewTransportHandler(context.Background(), r, io.Discard, &transportHandlerTestMockCrypt{})

	err := h.HandleTransport()
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected ErrUnexpectedEOF, got %v", err)
	}
}

func TestHandleTransport_InvalidLength(t *testing.T) {
	// length < 4 is invalid, then next prefix read returns EOF
	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.BigEndian, uint32(2))
	h := NewTransportHandler(context.Background(), buf, io.Discard, &transportHandlerTestMockCrypt{})

	err := h.HandleTransport()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF after invalid length, got %v", err)
	}
}

func TestHandleTransport_DecryptError(t *testing.T) {
	// decryption fails
	encrypted := []byte{9, 9, 9, 9}
	cryptErr := errors.New("decrypt failed")
	crypt := &transportHandlerTestMockCrypt{decryptFunc: func([]byte) ([]byte, error) {
		return nil, cryptErr
	}}

	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.BigEndian, uint32(len(encrypted)))
	buf.Write(encrypted)

	h := NewTransportHandler(context.Background(), buf, io.Discard, crypt)
	err := h.HandleTransport()
	if !errors.Is(err, cryptErr) {
		t.Fatalf("expected decrypt error, got %v", err)
	}
}

func TestHandleTransport_WriteError(t *testing.T) {
	// write to TUN fails
	encrypted := []byte{7, 7, 7, 7}
	plaintext := []byte("DATA")
	crypt := &transportHandlerTestMockCrypt{decryptFunc: func([]byte) ([]byte, error) {
		return plaintext, nil
	}}

	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.BigEndian, uint32(len(encrypted)))
	buf.Write(encrypted)

	werr := errors.New("write failed")
	h := NewTransportHandler(context.Background(), buf, &errWriter{err: werr}, crypt)

	err := h.HandleTransport()
	if !errors.Is(err, werr) {
		t.Fatalf("expected write error, got %v", err)
	}
}
