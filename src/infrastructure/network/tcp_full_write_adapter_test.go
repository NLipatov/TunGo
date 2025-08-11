package network

import (
	"errors"
	"io"
	"testing"
	"tungo/application"
)

// --- adapter mock ---

type writeStep struct {
	n   int
	err error
}

type fakeConnAdapter struct {
	writePlan []writeStep
	writeCall int

	readBuf []byte
	readN   int
	readErr error

	closed bool
}

func (f *fakeConnAdapter) Write(p []byte) (int, error) {
	if f.writeCall >= len(f.writePlan) {
		return 0, errors.New("unexpected write call")
	}
	step := f.writePlan[f.writeCall]
	f.writeCall++
	// written must not be greater than p
	if step.n > len(p) {
		return len(p), step.err
	}
	return step.n, step.err
}

func (f *fakeConnAdapter) Read(b []byte) (int, error) {
	n := f.readN
	if n > len(f.readBuf) {
		n = len(f.readBuf)
	}
	if n > len(b) {
		n = len(b)
	}
	copy(b, f.readBuf[:n])
	return n, f.readErr
}

func (f *fakeConnAdapter) Close() error {
	f.closed = true
	return nil
}

var _ application.ConnectionAdapter = (*fakeConnAdapter)(nil)

// --- tests ---

func TestTcpFullWriteAdapter_Write_FullSingleCall(t *testing.T) {
	f := &fakeConnAdapter{
		writePlan: []writeStep{{n: 8, err: nil}},
	}
	ta := NewTcpFullWriteAdapter(f)

	data := []byte("12345678")
	n, err := ta.Write(data)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != len(data) {
		t.Fatalf("want %d, got %d", len(data), n)
	}
	if f.writeCall != 1 {
		t.Fatalf("want 1 write call, got %d", f.writeCall)
	}
}

func TestTcpFullWriteAdapter_Write_PartialProgress(t *testing.T) {
	f := &fakeConnAdapter{
		writePlan: []writeStep{
			{n: 3, err: nil},
			{n: 2, err: nil},
			{n: 3, err: nil},
		},
	}
	ta := NewTcpFullWriteAdapter(f)

	data := []byte("abcdefgh")
	n, err := ta.Write(data)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != len(data) {
		t.Fatalf("want %d, got %d", len(data), n)
	}
	if f.writeCall != 3 {
		t.Fatalf("want 3 write calls, got %d", f.writeCall)
	}
}

func TestTcpFullWriteAdapter_Write_PartialThenError_ReturnsTotalAndErr(t *testing.T) {
	testErr := errors.New("boom")
	f := &fakeConnAdapter{
		writePlan: []writeStep{
			{n: 4, err: nil},     // progress is 4
			{n: 3, err: testErr}, // err after 3 more bytes => 7 bytes total
		},
	}
	ta := NewTcpFullWriteAdapter(f)

	data := []byte("123456789")
	n, err := ta.Write(data)
	if !errors.Is(err, testErr) {
		t.Fatalf("want err=%v, got %v", testErr, err)
	}
	if n != 7 { // total progress (off+n), not just last n
		t.Fatalf("want total=7, got %d", n)
	}
	if f.writeCall != 2 {
		t.Fatalf("want 2 write calls, got %d", f.writeCall)
	}
}

func TestTcpFullWriteAdapter_Write_ZeroWrite_ErrShortWrite(t *testing.T) {
	f := &fakeConnAdapter{
		writePlan: []writeStep{
			{n: 0, err: nil}, // 0-progress => io.ErrShortWrite
		},
	}
	ta := NewTcpFullWriteAdapter(f)

	data := []byte("xyz")
	n, err := ta.Write(data)
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("want io.ErrShortWrite, got %v", err)
	}
	if n != 0 {
		t.Fatalf("want total=0, got %d", n)
	}
	if f.writeCall != 1 {
		t.Fatalf("want 1 write call, got %d", f.writeCall)
	}
}

func TestTcpFullWriteAdapter_Write_EmptyInput_NoCalls(t *testing.T) {
	f := &fakeConnAdapter{
		writePlan: []writeStep{}, // should not be called
	}
	ta := NewTcpFullWriteAdapter(f)

	var data []byte
	n, err := ta.Write(data)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != 0 {
		t.Fatalf("want 0, got %d", n)
	}
	if f.writeCall != 0 {
		t.Fatalf("want 0 write calls, got %d", f.writeCall)
	}
}

func TestTcpFullWriteAdapter_Read_PassThrough(t *testing.T) {
	f := &fakeConnAdapter{
		readBuf: []byte("hello"),
		readN:   5,
	}
	ta := NewTcpFullWriteAdapter(f)

	buf := make([]byte, 10)
	n, err := ta.Read(buf)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != 5 || string(buf[:n]) != "hello" {
		t.Fatalf("read mismatch: n=%d data=%q", n, string(buf[:n]))
	}
}

func TestTcpFullWriteAdapter_Close_PassThrough(t *testing.T) {
	f := &fakeConnAdapter{}
	ta := NewTcpFullWriteAdapter(f)

	if err := ta.Close(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !f.closed {
		t.Fatalf("expected underlying adapter to be closed")
	}
}
