package handshake

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"io"
	"net"
	"testing"
)

// clientIOFakeConn — mock for application.ConnectionAdapter
type clientIOFakeConn struct {
	readBuf  *bytes.Buffer
	writeBuf *bytes.Buffer
	readErr  error
	writeErr error
}

func newClientIOFakeConn(readData []byte) *clientIOFakeConn {
	return &clientIOFakeConn{
		readBuf:  bytes.NewBuffer(readData),
		writeBuf: &bytes.Buffer{},
	}
}

func (f *clientIOFakeConn) Read(p []byte) (int, error) {
	if f.readErr != nil {
		return 0, f.readErr
	}
	return f.readBuf.Read(p)
}

func (f *clientIOFakeConn) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return f.writeBuf.Write(p)
}

func (f *clientIOFakeConn) Close() error { return nil }

func TestWriteClientHello_Success(t *testing.T) {
	ch := NewClientHello(4, net.ParseIP("10.0.0.1"),
		bytes.Repeat([]byte{1}, ed25519.PublicKeySize),
		bytes.Repeat([]byte{2}, curvePublicKeyLength),
		bytes.Repeat([]byte{3}, nonceLength),
	)
	conn := newClientIOFakeConn(nil)
	clientIO := NewDefaultClientIO(conn, NewEncrypter(nil, nil))
	if err := clientIO.WriteClientHello(&ch); err != nil {
		t.Fatalf("WriteClientHello failed: %v", err)
	}
	wantPlain, _ := ch.MarshalBinary()
	plain, ok, err := Obfuscator{}.Deobfuscate(conn.writeBuf.Bytes())
	if err != nil || !ok {
		t.Fatalf("written data not obfuscated or error: %v", err)
	}
	if !bytes.Equal(plain, wantPlain) {
		t.Errorf("payload mismatch")
	}
}

func TestWriteClientHello_MarshalError(t *testing.T) {
	// invalid version → MarshalBinary error
	ch := NewClientHello(0, net.ParseIP("x"), nil, nil, nil)
	conn := newClientIOFakeConn(nil)
	clientIO := NewDefaultClientIO(conn, NewEncrypter(nil, nil))
	if err := clientIO.WriteClientHello(&ch); err == nil {
		t.Error("expected Marshal error, got nil")
	}
}

func TestWriteClientHello_WriteError(t *testing.T) {
	ch := NewClientHello(4, net.ParseIP("10.0.0.1"),
		bytes.Repeat([]byte{1}, ed25519.PublicKeySize),
		bytes.Repeat([]byte{2}, curvePublicKeyLength),
		bytes.Repeat([]byte{3}, nonceLength),
	)
	conn := newClientIOFakeConn(nil)
	conn.writeErr = errors.New("write failure")
	clientIO := NewDefaultClientIO(conn, NewEncrypter(nil, nil))
	if err := clientIO.WriteClientHello(&ch); err == nil {
		t.Error("expected write error, got nil")
	}
}

func TestReadServerHello_Success(t *testing.T) {
	sig := bytes.Repeat([]byte{0xCC}, signatureLength)
	nonce := bytes.Repeat([]byte{0xDD}, nonceLength)
	curve := bytes.Repeat([]byte{0xEE}, curvePublicKeyLength)
	sh := NewServerHello(sig, nonce, curve)
	dataPlain, _ := sh.MarshalBinary()
	data, _ := Obfuscator{}.Obfuscate(dataPlain)

	conn := newClientIOFakeConn(data)
	clientIO := NewDefaultClientIO(conn, NewEncrypter(nil, nil))
	got, err := clientIO.ReadServerHello()
	if err != nil {
		t.Fatalf("ReadServerHello failed: %v", err)
	}
	if !bytes.Equal(got.signature, sig) {
		t.Error("Signature mismatch")
	}
	if !bytes.Equal(got.Nonce(), nonce) {
		t.Error("Nonce mismatch")
	}
	if !bytes.Equal(got.CurvePublicKey(), curve) {
		t.Error("CurvePublicKey mismatch")
	}
}

func TestReadServerHello_ReadError(t *testing.T) {
	conn := newClientIOFakeConn(nil)
	conn.readErr = io.ErrUnexpectedEOF
	clientIO := NewDefaultClientIO(conn, NewEncrypter(nil, nil))
	if _, err := clientIO.ReadServerHello(); err == nil {
		t.Error("expected read error, got nil")
	}
}

func TestReadServerHello_UnmarshalError(t *testing.T) {
	// too short → UnmarshalBinary error
	conn := newClientIOFakeConn([]byte{1, 2, 3})
	clientIO := NewDefaultClientIO(conn, NewEncrypter(nil, nil))
	if _, err := clientIO.ReadServerHello(); err == nil {
		t.Error("expected unmarshal error, got nil")
	}
}

func TestWriteClientSignature_Success(t *testing.T) {
	sig := bytes.Repeat([]byte{0xAB}, 64)
	conn := newClientIOFakeConn(nil)
	clientIO := NewDefaultClientIO(conn, NewEncrypter(nil, nil))
	if err := clientIO.WriteClientSignature(NewSignature(sig)); err != nil {
		t.Fatalf("WriteClientSignature failed: %v", err)
	}
	if !bytes.Equal(conn.writeBuf.Bytes(), sig) {
		t.Errorf("written signature = %x; want %x", conn.writeBuf.Bytes(), sig)
	}
}

func TestWriteClientSignature_MarshalError(t *testing.T) {
	sig := bytes.Repeat([]byte{0xAB}, 10)
	conn := newClientIOFakeConn(nil)
	clientIO := NewDefaultClientIO(conn, NewEncrypter(nil, nil))
	if err := clientIO.WriteClientSignature(NewSignature(sig)); err == nil {
		t.Error("expected marshal error, got nil")
	}
}

func TestWriteClientSignature_WriteError(t *testing.T) {
	sig := bytes.Repeat([]byte{0xAB}, 64)
	conn := newClientIOFakeConn(nil)
	conn.writeErr = errors.New("write fail")
	clientIO := NewDefaultClientIO(conn, NewEncrypter(nil, nil))
	if err := clientIO.WriteClientSignature(NewSignature(sig)); err == nil {
		t.Error("expected write error, got nil")
	}
}
