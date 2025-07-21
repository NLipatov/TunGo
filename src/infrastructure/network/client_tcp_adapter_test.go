package network

import (
	"encoding/binary"
	"errors"
	"reflect"
	"testing"
)

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
	adapter := NewClientTCPAdapter(mock)

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
	adapter := NewClientTCPAdapter(mock)

	data := make([]byte, 8)
	_, err := adapter.Write(data)
	if err == nil || err.Error() != "fail" {
		t.Errorf("expected error 'fail', got %v", err)
	}
}

func TestClientTCPAdapter_Write_PanicsOnShortSlice(t *testing.T) {
	mock := &ClientTCPAdapterMockConn{}
	adapter := NewClientTCPAdapter(mock)

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
	mock := &ClientTCPAdapterMockConn{readInput: []byte{1, 2, 3, 4, 5}, readReturnN: 5}
	adapter := NewClientTCPAdapter(mock)
	buf := make([]byte, 10)
	n, err := adapter.Read(buf)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if n != 5 {
		t.Errorf("want n=5, got %d", n)
	}
	if !reflect.DeepEqual(buf[:5], []byte{1, 2, 3, 4, 5}) {
		t.Errorf("read data mismatch: got %v", buf[:5])
	}
}

func TestClientTCPAdapter_Read_PropagatesError(t *testing.T) {
	mock := &ClientTCPAdapterMockConn{readReturnN: 0, readReturnE: errors.New("readfail")}
	adapter := NewClientTCPAdapter(mock)
	_, err := adapter.Read(make([]byte, 8))
	if err == nil || err.Error() != "readfail" {
		t.Errorf("expected error 'readfail', got %v", err)
	}
}

func TestClientTCPAdapter_Close_CallsUnderlying(t *testing.T) {
	mock := &ClientTCPAdapterMockConn{}
	adapter := NewClientTCPAdapter(mock)
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
	adapter := NewClientTCPAdapter(mock)
	err := adapter.Close()
	if err == nil || err.Error() != "closefail" {
		t.Errorf("expected error 'closefail', got %v", err)
	}
}
