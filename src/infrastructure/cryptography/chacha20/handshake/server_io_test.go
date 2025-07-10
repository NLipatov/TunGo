package handshake

import (
	"bytes"
	"errors"
	"io"
	"net"
	"testing"
)

type DefaultServerIOMockConn struct {
	ReadFunc  func([]byte) (int, error)
	WriteFunc func([]byte) (int, error)
	CloseFunc func() error
}

func (m *DefaultServerIOMockConn) Read(p []byte) (int, error)  { return m.ReadFunc(p) }
func (m *DefaultServerIOMockConn) Write(p []byte) (int, error) { return m.WriteFunc(p) }
func (m *DefaultServerIOMockConn) Close() error                { return m.CloseFunc() }

func generateValidClientHelloBinary() []byte {
	ip := net.IPv4(10, 10, 10, 10)
	edPub := make([]byte, curvePublicKeyLength)
	curvePub := make([]byte, curvePublicKeyLength)
	nonce := make([]byte, nonceLength)
	copy(edPub, bytes.Repeat([]byte{0x01}, curvePublicKeyLength))
	copy(curvePub, bytes.Repeat([]byte{0x02}, curvePublicKeyLength))
	copy(nonce, bytes.Repeat([]byte{0x03}, nonceLength))

	buf := make([]byte, lengthHeaderLength+len(ip)+curvePublicKeyLength+curvePublicKeyLength+nonceLength)
	buf[0] = 4
	buf[1] = uint8(len(ip))
	copy(buf[lengthHeaderLength:], ip)
	copy(buf[lengthHeaderLength+len(ip):], edPub)
	copy(buf[lengthHeaderLength+len(ip)+curvePublicKeyLength:], curvePub)
	copy(buf[lengthHeaderLength+len(ip)+curvePublicKeyLength+curvePublicKeyLength:], nonce)
	return buf
}

func generateInvalidClientHelloBinary() []byte {
	return []byte{0x00, 0x01}
}

func TestDefaultServerIO_ReceiveClientHello_Success(t *testing.T) {
	mockConn := &DefaultServerIOMockConn{
		ReadFunc: func(p []byte) (int, error) {
			bin := generateValidClientHelloBinary()
			copy(p, bin)
			return len(bin), nil
		},
	}
	sio := NewDefaultServerIO(mockConn)
	hello, err := sio.ReceiveClientHello()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hello.ipVersion != 4 {
		t.Fatalf("expected ipVersion 4, got %d", hello.ipVersion)
	}
}

func TestDefaultServerIO_ReceiveClientHello_ReadError(t *testing.T) {
	mockConn := &DefaultServerIOMockConn{
		ReadFunc: func(p []byte) (int, error) {
			return 0, io.ErrUnexpectedEOF
		},
	}
	sio := NewDefaultServerIO(mockConn)
	_, err := sio.ReceiveClientHello()
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected read error, got %v", err)
	}
}

func TestDefaultServerIO_ReceiveClientHello_UnmarshalError(t *testing.T) {
	mockConn := &DefaultServerIOMockConn{
		ReadFunc: func(p []byte) (int, error) {
			invalid := generateInvalidClientHelloBinary()
			copy(p, invalid)
			return len(invalid), nil
		},
	}
	sio := NewDefaultServerIO(mockConn)
	_, err := sio.ReceiveClientHello()
	if err == nil {
		t.Fatal("expected unmarshal error, got nil")
	}
}

func TestDefaultServerIO_SendServerHello_Success(t *testing.T) {
	mockConn := &DefaultServerIOMockConn{
		WriteFunc: func(p []byte) (int, error) {
			return len(p), nil
		},
	}
	sio := NewDefaultServerIO(mockConn)
	sig := bytes.Repeat([]byte{0xCC}, signatureLength)
	nonce := bytes.Repeat([]byte{0xDD}, nonceLength)
	curve := bytes.Repeat([]byte{0xEE}, curvePublicKeyLength)
	hello := NewServerHello(sig, nonce, curve)
	err := sio.SendServerHello(hello)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDefaultServerIO_ReadClientSignature_Success(t *testing.T) {
	valid := make([]byte, 64)
	mockConn := &DefaultServerIOMockConn{
		ReadFunc: func(p []byte) (int, error) {
			copy(p, valid)
			return len(valid), nil
		},
	}
	sio := NewDefaultServerIO(mockConn)
	_, err := sio.ReadClientSignature()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDefaultServerIO_ReadClientSignature_ReadError(t *testing.T) {
	mockConn := &DefaultServerIOMockConn{
		ReadFunc: func(p []byte) (int, error) {
			return 0, io.EOF
		},
	}
	sio := NewDefaultServerIO(mockConn)
	_, err := sio.ReadClientSignature()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF, got %v", err)
	}
}
