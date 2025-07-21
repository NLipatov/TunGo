package network

import (
	"encoding/binary"
	"errors"
	"reflect"
	"testing"
	"tungo/infrastructure/cryptography/chacha20"
)

type FrameConnMock struct {
	data   []byte
	offset int
}

func NewFrameConnMock(payload []byte) *FrameConnMock {
	frame := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(frame, uint32(len(payload)))
	copy(frame[4:], payload)
	return &FrameConnMock{data: frame}
}

func (m *FrameConnMock) Read(p []byte) (int, error) {
	if m.offset >= len(m.data) {
		return 0, errors.New("eof")
	}
	n := copy(p, m.data[m.offset:])
	m.offset += n
	return n, nil
}
func (m *FrameConnMock) Write(p []byte) (int, error) { return len(p), nil }
func (m *FrameConnMock) Close() error                { return nil }

// Mock for ConnectionAdapter with ClientTCPAdapter prefix
type ClientTCPAdapterMockConn struct {
	writeInput   []byte
	writeReturnN int
	writeReturnE error

	readInput   []byte
	readReturnN int
	readReturnE error

	closeCalled  bool
	closeReturnE error
}

func (m *ClientTCPAdapterMockConn) Write(p []byte) (int, error) {
	m.writeInput = append([]byte(nil), p...)
	return m.writeReturnN, m.writeReturnE
}
func (m *ClientTCPAdapterMockConn) Read(p []byte) (int, error) {
	copy(p, m.readInput)
	return m.readReturnN, m.readReturnE
}
func (m *ClientTCPAdapterMockConn) Close() error {
	m.closeCalled = true
	return m.closeReturnE
}

func TestClientTCPAdapter_Write_SetsLengthAndCallsWrite(t *testing.T) {
	mock := &ClientTCPAdapterMockConn{}
	enc := chacha20.NewDefaultTCPEncoder()
	adapter := NewClientTCPAdapter(mock, enc)

	data := []byte{0, 0, 0, 0, 10, 20, 30, 40, 50}
	wantLen := uint32(len(data[4:]))

	n, err := adapter.Write(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected n=0, got %d", n)
	}

	if mock.writeInput == nil {
		t.Fatal("Write was not called on mock")
	}
	gotLen := binary.BigEndian.Uint32(mock.writeInput[:4])
	if gotLen != wantLen {
		t.Errorf("first 4 bytes = %d, want %d", gotLen, wantLen)
	}
	// Check that remaining bytes match original (content after first 4)
	if !reflect.DeepEqual(mock.writeInput[4:], data[4:]) {
		t.Errorf("payload after length mismatch: got %v, want %v", mock.writeInput[4:], data[4:])
	}
}

func TestClientTCPAdapter_Write_PropagatesError(t *testing.T) {
	mock := &ClientTCPAdapterMockConn{writeReturnN: 0, writeReturnE: errors.New("fail")}
	enc := chacha20.NewDefaultTCPEncoder()
	adapter := NewClientTCPAdapter(mock, enc)

	data := make([]byte, 8)
	_, err := adapter.Write(data)
	if err == nil || err.Error() != "fail" {
		t.Errorf("expected error 'fail', got %v", err)
	}
}

func TestClientTCPAdapter_Write_PanicsOnShortSlice(t *testing.T) {
	mock := &ClientTCPAdapterMockConn{}
	enc := chacha20.NewDefaultTCPEncoder()
	adapter := NewClientTCPAdapter(mock, enc)

	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic on len(p) < 4, got none")
		}
	}()
	data := []byte{1, 2, 3}
	_, _ = adapter.Write(data) // should panic
}

func TestClientTCPAdapter_Read_CallsUnderlying(t *testing.T) {
	payload := []byte{1, 2, 3, 4, 5, 6}
	frameMockConn := NewFrameConnMock(payload)
	enc := chacha20.NewDefaultTCPEncoder()
	adapter := NewClientTCPAdapter(frameMockConn, enc)

	buf := make([]byte, 32)
	n, err := adapter.Read(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(payload) {
		t.Errorf("want n=%d, got %d", len(payload), n)
	}
	if !reflect.DeepEqual(buf[:n], payload) {
		t.Errorf("read data mismatch: got %v, want %v", buf[:n], payload)
	}
}

func TestClientTCPAdapter_Read_PropagatesError(t *testing.T) {
	mock := &ClientTCPAdapterMockConn{readReturnN: 0, readReturnE: errors.New("readfail")}
	enc := chacha20.NewDefaultTCPEncoder()
	adapter := NewClientTCPAdapter(mock, enc)
	_, err := adapter.Read(make([]byte, 8))
	if err == nil || err.Error() != "failed to read length prefix: readfail" {
		t.Errorf("expected error 'readfail', got %v", err)
	}
}

func TestClientTCPAdapter_Close_CallsUnderlying(t *testing.T) {
	mock := &ClientTCPAdapterMockConn{}
	enc := chacha20.NewDefaultTCPEncoder()
	adapter := NewClientTCPAdapter(mock, enc)
	err := adapter.Close()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !mock.closeCalled {
		t.Errorf("close was not called on mock")
	}
}

func TestClientTCPAdapter_Close_PropagatesError(t *testing.T) {
	mock := &ClientTCPAdapterMockConn{closeReturnE: errors.New("closefail")}
	enc := chacha20.NewDefaultTCPEncoder()
	adapter := NewClientTCPAdapter(mock, enc)
	err := adapter.Close()
	if err == nil || err.Error() != "closefail" {
		t.Errorf("expected error 'closefail', got %v", err)
	}
}
