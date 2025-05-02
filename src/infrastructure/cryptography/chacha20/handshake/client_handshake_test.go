package handshake

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"

	"golang.org/x/crypto/curve25519"
	"tungo/settings"
)

// ClientHandshakeFakeIO implements ClientIO for ClientHandshake tests.
type ClientHandshakeFakeIO struct {
	wroteHello     bool
	helloArg       ClientHello
	writeHelloErr  error
	readHello      ServerHello
	readHelloErr   error
	wroteSignature bool
	signatureArg   Signature
	writeSigErr    error
}

func (f *ClientHandshakeFakeIO) WriteClientHello(h ClientHello) error {
	f.wroteHello = true
	f.helloArg = h
	return f.writeHelloErr
}
func (f *ClientHandshakeFakeIO) ReadServerHello() (ServerHello, error) {
	return f.readHello, f.readHelloErr
}
func (f *ClientHandshakeFakeIO) WriteClientSignature(s Signature) error {
	f.wroteSignature = true
	f.signatureArg = s
	return f.writeSigErr
}

// ClientHandshakeFakeCrypto implements Crypto for ClientHandshake tests.
type ClientHandshakeFakeCrypto struct {
	verifyOK bool
	signOut  []byte
}

func (c *ClientHandshakeFakeCrypto) Verify(_ ed25519.PublicKey, _, _ []byte) bool {
	return c.verifyOK
}
func (c *ClientHandshakeFakeCrypto) Sign(_ ed25519.PrivateKey, _ []byte) []byte {
	return c.signOut
}
func (c *ClientHandshakeFakeCrypto) GenerateEd25519KeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return nil, nil, nil
}
func (c *ClientHandshakeFakeCrypto) GenerateX25519KeyPair() ([]byte, [32]byte, error) {
	return nil, [32]byte{}, nil
}
func (c *ClientHandshakeFakeCrypto) GenerateRandomBytesArray(_ int) []byte { return nil }
func (c *ClientHandshakeFakeCrypto) GenerateChaCha20KeysServerside(_, _ []byte, _ Hello) ([32]byte, []byte, []byte, error) {
	return [32]byte{}, nil, nil, nil
}
func (c *ClientHandshakeFakeCrypto) GenerateChaCha20KeysClientside(_, _ []byte, _ Hello) ([]byte, []byte, [32]byte, error) {
	return nil, nil, [32]byte{}, nil
}

func TestSendClientHello(t *testing.T) {
	io := &ClientHandshakeFakeIO{}
	ch := NewClientHandshake(nil, io, nil)
	s := settings.ConnectionSettings{InterfaceAddress: "1.2.3.4"}
	edPub := make([]byte, ed25519.PublicKeySize)
	sessPub := make([]byte, curve25519.ScalarSize)
	salt := make([]byte, nonceLength)

	// success
	if err := ch.SendClientHello(s, edPub, sessPub, salt); err != nil {
		t.Fatalf("SendClientHello failed: %v", err)
	}
	if !io.wroteHello {
		t.Fatal("WriteClientHello was not called")
	}
	if io.helloArg.ipVersion != 4 || io.helloArg.ipAddress != s.InterfaceAddress {
		t.Errorf("unexpected helloArg %+v", io.helloArg)
	}

	// write error
	io = &ClientHandshakeFakeIO{writeHelloErr: errors.New("boom")}
	ch = NewClientHandshake(nil, io, nil)
	if err := ch.SendClientHello(s, edPub, sessPub, salt); err == nil {
		t.Error("expected error, got nil")
	}
}

func TestReceiveServerHello(t *testing.T) {
	want := ServerHello{signature: []byte{1, 2, 3}, nonce: []byte{9}, curvePublicKey: []byte{8}}
	io := &ClientHandshakeFakeIO{readHello: want}
	ch := NewClientHandshake(nil, io, nil)

	// success
	got, err := ch.ReceiveServerHello()
	if err != nil {
		t.Fatalf("ReceiveServerHello error: %v", err)
	}
	// we must compare slice‑fields manually
	if !bytes.Equal(got.signature, want.signature) ||
		!bytes.Equal(got.nonce, want.nonce) ||
		!bytes.Equal(got.curvePublicKey, want.curvePublicKey) {
		t.Errorf("got %+v, want %+v", got, want)
	}

	// read error
	io = &ClientHandshakeFakeIO{readHelloErr: errors.New("rfail")}
	ch = NewClientHandshake(nil, io, nil)
	if _, err := ch.ReceiveServerHello(); err == nil {
		t.Error("expected read error, got nil")
	}
}

func TestSendSignature(t *testing.T) {
	curvePub := make([]byte, curve25519.ScalarSize)
	nonce := make([]byte, 32)
	sig := make([]byte, 32)
	hello := ServerHello{signature: sig, nonce: nonce, curvePublicKey: curvePub}

	edPub, edPriv, _ := ed25519.GenerateKey(rand.Reader)
	sessPub := make([]byte, curve25519.ScalarSize)
	salt := []byte("clientsalt-012345678901234567890123")[:nonceLength]

	// 1) invalid Ed25519 key length
	io1 := &ClientHandshakeFakeIO{}
	ch1 := NewClientHandshake(nil, io1, &ClientHandshakeFakeCrypto{})
	if err := ch1.SendSignature(edPub[:1], edPriv, sessPub, hello, salt); err == nil {
		t.Error("expected invalid Ed25519 key length error")
	}

	// 2) invalid X25519 session public key length
	io2 := &ClientHandshakeFakeIO{}
	ch2 := NewClientHandshake(nil, io2, &ClientHandshakeFakeCrypto{})
	if err := ch2.SendSignature(edPub, edPriv, []byte{1, 2}, hello, salt); err == nil {
		t.Error("expected invalid X25519 session public key length error")
	}

	// 3) verify fails
	io3 := &ClientHandshakeFakeIO{}
	ch3 := NewClientHandshake(nil, io3, &ClientHandshakeFakeCrypto{verifyOK: false})
	if err := ch3.SendSignature(edPub, edPriv, sessPub, hello, salt); err == nil {
		t.Error("expected server failed signature check")
	}

	// 4) write signature error
	io4 := &ClientHandshakeFakeIO{writeSigErr: errors.New("wfail")}
	crypto4 := &ClientHandshakeFakeCrypto{verifyOK: true, signOut: []byte("clientsig-012345678901234567890123456789012345678901234567")}
	ch4 := NewClientHandshake(nil, io4, crypto4)
	if err := ch4.SendSignature(edPub, edPriv, sessPub, hello, salt); err == nil {
		t.Error("expected write signature error")
	}

	// 5) success
	io5 := &ClientHandshakeFakeIO{}
	crypto5 := &ClientHandshakeFakeCrypto{verifyOK: true, signOut: []byte("clientsig-012345678901234567890123456789012345678901234567")}
	ch5 := NewClientHandshake(nil, io5, crypto5)
	if err := ch5.SendSignature(edPub, edPriv, sessPub, hello, salt); err != nil {
		t.Fatalf("SendSignature failed: %v", err)
	}
	if !io5.wroteSignature {
		t.Fatal("WriteClientSignature was not called")
	}
	if !bytes.Equal(io5.signatureArg.data, crypto5.signOut) {
		t.Errorf("written signature = %x; want %x", io5.signatureArg.data, crypto5.signOut)
	}
}
