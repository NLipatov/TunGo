//go:build darwin

package utun

import (
	"encoding/binary"
	"errors"
	"reflect"
	"syscall"
	"testing"
)

type fakeTun struct {
	// Read simulation
	readPayload []byte
	readSize    int
	readErr     error
	// Write simulation
	writtenBuf [][]byte
	writeOff   int
	writeErr   error
	// Close simulation
	closeErr error
	closed   bool
}

func (f *fakeTun) Name() (string, error) { panic("not implemented") }

func (f *fakeTun) Read(bufs [][]byte, sizes []int, _ int) (int, error) {
	if f.readErr != nil {
		return 0, f.readErr
	}
	// Scatter-gather: bufs[0] = header sink, bufs[1] = payload destination
	if len(bufs) > 0 && len(f.readPayload) >= 4 {
		copy(bufs[0], f.readPayload[:4])
	}
	if len(bufs) > 1 && f.readSize > 0 {
		copy(bufs[1], f.readPayload[4:4+f.readSize])
	}
	sizes[0] = f.readSize
	return f.readSize, nil
}

func (f *fakeTun) Write(bufs [][]byte, offset int) (int, error) {
	// capture written data
	f.writtenBuf = make([][]byte, len(bufs))
	for i := range bufs {
		f.writtenBuf[i] = append([]byte(nil), bufs[i]...)
	}
	f.writeOff = offset
	return len(bufs[0]) - offset, f.writeErr
}

func (f *fakeTun) Close() error {
	f.closed = true
	return f.closeErr
}

func TestRead_Success(t *testing.T) {
	payload := []byte{0x11, 0x22, 0x33}
	ft := &fakeTun{
		// 4-byte header + payload
		readPayload: append(make([]byte, 4), payload...),
		readSize:    len(payload),
	}
	adapter := NewDarwinTunDevice(ft)

	out := make([]byte, len(payload))
	n, err := adapter.Read(out)
	if err != nil {
		t.Fatalf("Read returned unexpected error: %v", err)
	}
	if n != len(payload) {
		t.Fatalf("Read returned length %d, want %d", n, len(payload))
	}
	if !reflect.DeepEqual(out, payload) {
		t.Fatalf("Read payload = %v, want %v", out, payload)
	}
}

func TestRead_ErrFromDevice(t *testing.T) {
	wantErr := errors.New("read fail")
	ft := &fakeTun{readErr: wantErr}
	adapter := NewDarwinTunDevice(ft)

	_, err := adapter.Read(make([]byte, 10))
	if !errors.Is(err, wantErr) {
		t.Fatalf("Read error = %v, want %v", err, wantErr)
	}
}

func TestRead_DestinationTooSmall(t *testing.T) {
	ft := &fakeTun{}
	adapter := NewDarwinTunDevice(ft)

	_, err := adapter.Read(nil)
	if err == nil || err.Error() != "destination slice too small" {
		t.Fatalf("Read error = %v, want destination slice too small", err)
	}
}

func TestWrite_Success_IPv4(t *testing.T) {
	// first nibble = 4 indicates IPv4
	payload := []byte{0x45, 0xAA, 0xBB}
	ft := &fakeTun{}
	adapter := NewDarwinTunDevice(ft)

	n, err := adapter.Write(payload)
	if err != nil {
		t.Fatalf("Write returned unexpected error: %v", err)
	}
	if n != len(payload) {
		t.Fatalf("Write returned length %d, want %d", n, len(payload))
	}

	// Scatter-gather: writtenBuf[0] = 4-byte AF header, writtenBuf[1] = payload
	if len(ft.writtenBuf) != 2 {
		t.Fatalf("expected 2 iovecs, got %d", len(ft.writtenBuf))
	}

	// verify IPv4 family header
	wantFam := make([]byte, 4)
	binary.BigEndian.PutUint32(wantFam, syscall.AF_INET)
	if !reflect.DeepEqual(ft.writtenBuf[0], wantFam) {
		t.Fatalf("family header = %v, want %v", ft.writtenBuf[0], wantFam)
	}
	if !reflect.DeepEqual(ft.writtenBuf[1], payload) {
		t.Fatalf("payload = %v, want %v", ft.writtenBuf[1], payload)
	}
	if ft.writeOff != 0 {
		t.Fatalf("write offset = %d, want 0", ft.writeOff)
	}
}

func TestWrite_Success_IPv6(t *testing.T) {
	// first nibble = 6 indicates IPv6
	payload := []byte{0x60, 0xDE, 0xAD}
	ft := &fakeTun{}
	adapter := NewDarwinTunDevice(ft)

	_, err := adapter.Write(payload)
	if err != nil {
		t.Fatalf("Write error = %v", err)
	}

	// verify IPv6 family header (scatter-gather: writtenBuf[0] = header)
	wantFam := make([]byte, 4)
	binary.BigEndian.PutUint32(wantFam, syscall.AF_INET6)
	if !reflect.DeepEqual(ft.writtenBuf[0], wantFam) {
		t.Fatalf("family header = %v, want %v", ft.writtenBuf[0], wantFam)
	}
}

func TestWrite_EmptyPacket(t *testing.T) {
	ft := &fakeTun{}
	adapter := NewDarwinTunDevice(ft)

	_, err := adapter.Write(nil)
	if err == nil || err.Error() != "empty packet" {
		t.Fatalf("Write error = %v, want empty packet", err)
	}
}

func TestWrite_DeviceError(t *testing.T) {
	payload := []byte{0x45, 0xAA}
	wantErr := errors.New("write fail")
	ft := &fakeTun{writeErr: wantErr}
	adapter := NewDarwinTunDevice(ft)

	_, err := adapter.Write(payload)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Write error = %v, want %v", err, wantErr)
	}
}

func TestClose(t *testing.T) {
	ft := &fakeTun{}
	adapter := NewDarwinTunDevice(ft)
	err := adapter.Close()
	if err != nil {
		t.Fatalf("Close error = %v, want nil", err)
	}
	if !ft.closed {
		t.Fatal("Close: underlying device.Close was not called")
	}
}

func TestClose_Error(t *testing.T) {
	wantErr := errors.New("close fail")
	ft := &fakeTun{closeErr: wantErr}
	adapter := NewDarwinTunDevice(ft)
	err := adapter.Close()
	if !errors.Is(err, wantErr) {
		t.Fatalf("Close error = %v, want %v", err, wantErr)
	}
	if !ft.closed {
		t.Fatal("Close: underlying Close should have been called even on error")
	}
}
