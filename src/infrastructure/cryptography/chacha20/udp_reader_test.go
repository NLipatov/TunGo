package chacha20

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

type udpTestErrReader struct{ err error }

func (r *udpTestErrReader) Read([]byte) (int, error) {
	return 0, r.err
}

type udpTestPartialErrReader struct {
	data []byte
	done bool
}

func (r *udpTestPartialErrReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.done = true
	return n, errors.New("partial error")
}

func TestNewUdpReader(t *testing.T) {
	if NewUdpReader(bytes.NewReader(nil)) == nil {
		t.Fatal("constructor returned nil")
	}
}

func TestUdpReader_Read_Success(t *testing.T) {
	// 5-bytes long payload
	payload := []byte("WORLD")
	ur := NewUdpReader(bytes.NewReader(payload))

	// buffer is 17 bytes, where 5 bytes is payload and 12 bytes is reserved for nonce
	buf := make([]byte, 12+len(payload))
	// will return 17 (12 bytes nonce and 5 bytes payload)
	n, err := ur.Read(buf)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if n-12 != len(payload) {
		t.Errorf("expected %d bytes, got %d", len(payload), n)
	}

	// first 4 bytes of package contains length of package (which is nonce header + payload)
	prefix := binary.BigEndian.Uint32(buf[:4])
	if want := uint32(n); prefix != want {
		t.Errorf("expected prefix %d, got %d", want, prefix)
	}
	if !bytes.Equal(buf[12:], payload) {
		t.Errorf("payload mismatch: %q vs %q", payload, buf[12:])
	}
}

func TestUdpReader_Read_ErrorImmediate(t *testing.T) {
	err := io.ErrUnexpectedEOF
	ur := NewUdpReader(&udpTestErrReader{err: err})
	buf := make([]byte, 20)

	n, got := ur.Read(buf)
	if n != 0 {
		t.Errorf("expected 0 bytes on error, got %d", n)
	}
	if !errors.Is(got, err) {
		t.Errorf("expected error %v, got %v", err, got)
	}
}

func TestUdpReader_Read_PartialThenError(t *testing.T) {
	payload := []byte("XY")
	ur := NewUdpReader(&udpTestPartialErrReader{data: payload})

	buf := make([]byte, 12+len(payload))
	n, got := ur.Read(buf)
	if n != 0 {
		t.Errorf("expected 0 bytes when reader returns data+error, got %d", n)
	}
	if got == nil || got.Error() != "partial error" {
		t.Errorf("expected \"partial error\", got %v", got)
	}
}
