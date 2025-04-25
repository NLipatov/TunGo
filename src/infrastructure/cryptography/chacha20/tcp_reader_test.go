package chacha20

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

type tcpTestErrReader struct{ err error }

func (r *tcpTestErrReader) Read([]byte) (int, error) {
	return 0, r.err
}

type tcpTestPartialErrReader struct {
	data []byte
	done bool
}

func (r *tcpTestPartialErrReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.done = true
	return n, errors.New("partial error")
}

func TestNewTcpReader(t *testing.T) {
	if NewTcpReader(bytes.NewReader(nil)) == nil {
		t.Fatal("constructor returned nil")
	}
}

func TestTcpReader_Read_Success(t *testing.T) {
	payload := []byte("HELLO")
	tr := NewTcpReader(bytes.NewReader(payload))

	// buffer must hold 4-byte length prefix + payload
	buf := make([]byte, 4+len(payload))
	n, err := tr.Read(buf)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if n != len(payload) {
		t.Errorf("expected %d bytes, got %d", len(payload), n)
	}

	// verify length prefix
	total := binary.BigEndian.Uint32(buf[:4])
	if want := uint32(n + 4); total != want {
		t.Errorf("expected prefix %d, got %d", want, total)
	}
	if !bytes.Equal(buf[4:], payload) {
		t.Errorf("payload mismatch: %q vs %q", payload, buf[4:])
	}
}

func TestTcpReader_Read_ErrorImmediate(t *testing.T) {
	err := io.ErrUnexpectedEOF
	tr := NewTcpReader(&tcpTestErrReader{err: err})
	buf := make([]byte, 10)

	n, got := tr.Read(buf)
	if n != 0 {
		t.Errorf("expected 0 bytes on error, got %d", n)
	}
	if !errors.Is(got, err) {
		t.Errorf("expected error %v, got %v", err, got)
	}
}

func TestTcpReader_Read_PartialThenError(t *testing.T) {
	payload := []byte("AB")
	tr := NewTcpReader(&tcpTestPartialErrReader{data: payload})

	buf := make([]byte, 4+len(payload))
	n, got := tr.Read(buf)
	if n != 0 {
		t.Errorf("expected 0 bytes when reader returns data+error, got %d", n)
	}
	if got == nil || got.Error() != "partial error" {
		t.Errorf("expected \"partial error\", got %v", got)
	}
}
