package tun_adapters

import (
	"encoding/binary"
	"errors"
	"os"
	"reflect"
	"syscall"
	"testing"
	"tungo/infrastructure/settings"

	"golang.zx2c4.com/wireguard/tun"
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

func (f *fakeTun) File() *os.File           { panic("not implemented") }
func (f *fakeTun) MTU() (int, error)        { panic("not implemented") }
func (f *fakeTun) Name() (string, error)    { panic("not implemented") }
func (f *fakeTun) Events() <-chan tun.Event { panic("not implemented") }
func (f *fakeTun) BatchSize() int           { panic("not implemented") }

func (f *fakeTun) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	if f.readErr != nil {
		return 0, f.readErr
	}
	// copy payload after 4-byte utun header
	copy(bufs[0][offset:], f.readPayload[offset:offset+f.readSize])
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
	adapter := NewWgTunAdapter(ft)

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
	adapter := NewWgTunAdapter(ft)

	_, err := adapter.Read(make([]byte, 10))
	if !errors.Is(err, wantErr) {
		t.Fatalf("Read error = %v, want %v", err, wantErr)
	}
}

func TestRead_DestinationTooSmall(t *testing.T) {
	payload := []byte{1, 2, 3, 4, 5}
	ft := &fakeTun{
		readPayload: append(make([]byte, 4), payload...),
		readSize:    len(payload),
	}
	adapter := NewWgTunAdapter(ft)

	_, err := adapter.Read(make([]byte, len(payload)-1))
	if err == nil || err.Error() != "destination slice too small" {
		t.Fatalf("Read error = %v, want destination slice too small", err)
	}
}

func TestWrite_Success_IPv4(t *testing.T) {
	// first nibble = 4 indicates IPv4
	payload := []byte{0x45, 0xAA, 0xBB}
	ft := &fakeTun{}
	adapter := NewWgTunAdapter(ft)

	n, err := adapter.Write(payload)
	if err != nil {
		t.Fatalf("Write returned unexpected error: %v", err)
	}
	if n != len(payload) {
		t.Fatalf("Write returned length %d, want %d", n, len(payload))
	}

	wbuf := ft.writtenBuf[0]
	if len(wbuf) != len(payload)+4 {
		t.Fatalf("written buffer len = %d, want %d", len(wbuf), len(payload)+4)
	}

	// verify IPv4 family header
	wantFam := make([]byte, 4)
	binary.BigEndian.PutUint32(wantFam, syscall.AF_INET)
	if !reflect.DeepEqual(wbuf[:4], wantFam) {
		t.Fatalf("family header = %v, want %v", wbuf[:4], wantFam)
	}
	if !reflect.DeepEqual(wbuf[4:], payload) {
		t.Fatalf("payload = %v, want %v", wbuf[4:], payload)
	}
	if ft.writeOff != 4 {
		t.Fatalf("write offset = %d, want 4", ft.writeOff)
	}
}

func TestWrite_Success_IPv6(t *testing.T) {
	// first nibble = 6 indicates IPv6
	payload := []byte{0x60, 0xDE, 0xAD}
	ft := &fakeTun{}
	adapter := NewWgTunAdapter(ft)

	_, err := adapter.Write(payload)
	if err != nil {
		t.Fatalf("Write error = %v", err)
	}

	// verify IPv6 family header
	wbuf := ft.writtenBuf[0]
	wantFam := make([]byte, 4)
	binary.BigEndian.PutUint32(wantFam, syscall.AF_INET6)
	if !reflect.DeepEqual(wbuf[:4], wantFam) {
		t.Fatalf("family header = %v, want %v", wbuf[:4], wantFam)
	}
}

func TestWrite_EmptyPacket(t *testing.T) {
	ft := &fakeTun{}
	adapter := NewWgTunAdapter(ft)

	_, err := adapter.Write(nil)
	if err == nil || err.Error() != "empty packet" {
		t.Fatalf("Write error = %v, want empty packet", err)
	}
}

func TestWrite_TooLargePacket(t *testing.T) {
	// payload larger than MaxPacketLengthBytes-4
	tooBig := make([]byte, settings.MTU+UTUNHeaderSize)
	ft := &fakeTun{}
	adapter := NewWgTunAdapter(ft)

	_, err := adapter.Write(tooBig)
	if err == nil || err.Error() != "packet exceeds max size" {
		t.Fatalf("Write error = %v, want packet exceeds max size", err)
	}
}

func TestWrite_DeviceError(t *testing.T) {
	payload := []byte{0x45, 0xAA}
	wantErr := errors.New("write fail")
	ft := &fakeTun{writeErr: wantErr}
	adapter := NewWgTunAdapter(ft)

	_, err := adapter.Write(payload)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Write error = %v, want %v", err, wantErr)
	}
}

func TestClose(t *testing.T) {
	ft := &fakeTun{}
	adapter := NewWgTunAdapter(ft)
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
	adapter := NewWgTunAdapter(ft)
	err := adapter.Close()
	if !errors.Is(err, wantErr) {
		t.Fatalf("Close error = %v, want %v", err, wantErr)
	}
	if !ft.closed {
		t.Fatal("Close: underlying Close should have been called even on error")
	}
}
