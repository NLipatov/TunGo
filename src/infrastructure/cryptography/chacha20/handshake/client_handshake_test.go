package handshake

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"strings"
	"testing"

	"golang.org/x/crypto/curve25519"
	"tungo/infrastructure/settings"
)

// --- your fakes (kept) ---

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

type ClientHandshakeFakeCrypto struct {
	verifyOK   bool
	signOut    []byte
	signCalled bool
}

func (c *ClientHandshakeFakeCrypto) Verify(_ ed25519.PublicKey, _, _ []byte) bool {
	return c.verifyOK
}
func (c *ClientHandshakeFakeCrypto) Sign(_ ed25519.PrivateKey, _ []byte) []byte {
	c.signCalled = true
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
func (c *ClientHandshakeFakeCrypto) DeriveKey(_ []byte, _ []byte, _ []byte) ([]byte, error) {
	return nil, nil
}

// Capturing crypto to assert exact payloads passed to Verify and Sign.
type CapturingCrypto struct {
	verifyOK bool
	signOut  []byte

	gotVerifyPub ed25519.PublicKey
	gotVerifyMsg []byte
	gotVerifySig []byte

	gotSignPriv ed25519.PrivateKey
	gotSignMsg  []byte
}

func (c *CapturingCrypto) Verify(pub ed25519.PublicKey, msg, sig []byte) bool {
	c.gotVerifyPub = append(ed25519.PublicKey(nil), pub...)
	c.gotVerifyMsg = append([]byte(nil), msg...)
	c.gotVerifySig = append([]byte(nil), sig...)
	return c.verifyOK
}
func (c *CapturingCrypto) Sign(priv ed25519.PrivateKey, msg []byte) []byte {
	c.gotSignPriv = append(ed25519.PrivateKey(nil), priv...)
	c.gotSignMsg = append([]byte(nil), msg...)
	return c.signOut
}
func (c *CapturingCrypto) GenerateEd25519KeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return nil, nil, nil
}
func (c *CapturingCrypto) GenerateX25519KeyPair() ([]byte, [32]byte, error) {
	return nil, [32]byte{}, nil
}
func (c *CapturingCrypto) GenerateRandomBytesArray(_ int) []byte { return nil }
func (c *CapturingCrypto) GenerateChaCha20KeysServerside(_, _ []byte, _ Hello) ([32]byte, []byte, []byte, error) {
	return [32]byte{}, nil, nil, nil
}
func (c *CapturingCrypto) GenerateChaCha20KeysClientside(_, _ []byte, _ Hello) ([]byte, []byte, [32]byte, error) {
	return nil, nil, [32]byte{}, nil
}
func (c *CapturingCrypto) DeriveKey(_ []byte, _ []byte, _ []byte) ([]byte, error) {
	return nil, nil
}

// --- existing tests (kept, maybe renamed) ---

func TestSendClientHello_IPv4_Success_And_WriteError(t *testing.T) {
	io := &ClientHandshakeFakeIO{}
	ch := NewClientHandshake(nil, io, nil)
	s := settings.Settings{
		InterfaceAddress: "1.2.3.4",
		ConnectionIP:     "4.3.2.1",
	}
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
	if io.helloArg.ipVersion != 4 || !bytes.Equal(io.helloArg.ipAddress, net.ParseIP(s.InterfaceAddress)) {
		t.Errorf("unexpected helloArg %+v", io.helloArg)
	}

	// write error
	io = &ClientHandshakeFakeIO{writeHelloErr: errors.New("boom")}
	ch = NewClientHandshake(nil, io, nil)
	if err := ch.SendClientHello(s, edPub, sessPub, salt); err == nil {
		t.Error("expected error, got nil")
	}
}

func TestSendClientHello_IPv6_Success(t *testing.T) {
	io := &ClientHandshakeFakeIO{}
	ch := NewClientHandshake(nil, io, nil)
	s := settings.Settings{
		InterfaceAddress: "fd00::10",
		ConnectionIP:     "fd00::1",
	}
	edPub := make([]byte, ed25519.PublicKeySize)
	sessPub := make([]byte, curve25519.ScalarSize)
	salt := make([]byte, nonceLength)

	if err := ch.SendClientHello(s, edPub, sessPub, salt); err != nil {
		t.Fatalf("SendClientHello failed: %v", err)
	}
	if !io.wroteHello {
		t.Fatal("WriteClientHello was not called")
	}
	if io.helloArg.ipVersion != 6 {
		t.Errorf("expected IPv6 ipVersion=6, got %v", io.helloArg.ipVersion)
	}
}

func TestSendClientHello_InvalidConnectionIP_Error(t *testing.T) {
	io := &ClientHandshakeFakeIO{}
	ch := NewClientHandshake(nil, io, nil)
	s := settings.Settings{
		InterfaceAddress: "1.2.3.4",
		ConnectionIP:     "not-an-ip",
	}
	edPub := make([]byte, ed25519.PublicKeySize)
	sessPub := make([]byte, curve25519.ScalarSize)
	salt := make([]byte, nonceLength)

	if err := ch.SendClientHello(s, edPub, sessPub, salt); err == nil {
		t.Fatal("expected parse error for ConnectionIP")
	}
}

func TestSendClientHello_InvalidInterfaceIP_AllowsNil(t *testing.T) {
	// net.ParseIP(InterfaceAddress) may return nil; function should still pass hello to IO.
	io := &ClientHandshakeFakeIO{}
	ch := NewClientHandshake(nil, io, nil)
	s := settings.Settings{
		InterfaceAddress: "999.999.999.999", // invalid, ParseIP -> nil
		ConnectionIP:     "4.3.2.1",
	}
	edPub := make([]byte, ed25519.PublicKeySize)
	sessPub := make([]byte, curve25519.ScalarSize)
	salt := make([]byte, nonceLength)

	if err := ch.SendClientHello(s, edPub, sessPub, salt); err != nil {
		t.Fatalf("SendClientHello failed: %v", err)
	}
	if !io.wroteHello {
		t.Fatal("WriteClientHello was not called")
	}
	if io.helloArg.ipAddress != nil {
		t.Errorf("expected nil ipAddress in hello, got %v", io.helloArg.ipAddress)
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
	if !bytes.Equal(got.signature, want.signature) ||
		!bytes.Equal(got.nonce, want.nonce) ||
		!bytes.Equal(got.curvePublicKey, want.curvePublicKey) {
		t.Errorf("got %+v, want %+v", got, want)
	}

	// read error + message prefix check
	io = &ClientHandshakeFakeIO{readHelloErr: errors.New("rfail")}
	ch = NewClientHandshake(nil, io, nil)
	if _, err := ch.ReceiveServerHello(); err == nil {
		t.Error("expected read error, got nil")
	} else if !strings.Contains(err.Error(), "client handshake: could not receive hello from server") {
		t.Errorf("error message prefix not found: %v", err)
	}
}

func TestSendSignature_AllBranchesAndPayloads(t *testing.T) {
	curvePub := make([]byte, curve25519.ScalarSize)
	for i := range curvePub {
		curvePub[i] = byte(i + 1)
	}
	nonce := []byte("0123456789abcdef0123456789abcdef") // 32 bytes
	sig := []byte("server-sig")
	hello := ServerHello{signature: sig, nonce: nonce, curvePublicKey: curvePub}

	edPub, edPriv, _ := ed25519.GenerateKey(rand.Reader)
	sessPub := make([]byte, curve25519.ScalarSize)
	for i := range sessPub {
		sessPub[i] = byte(0xAA + i)
	}
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

	// 3) verify fails: Sign must NOT be called, signature must NOT be written
	io3 := &ClientHandshakeFakeIO{}
	crypto3 := &ClientHandshakeFakeCrypto{verifyOK: false}
	ch3 := NewClientHandshake(nil, io3, crypto3)
	if err := ch3.SendSignature(edPub, edPriv, sessPub, hello, salt); err == nil {
		t.Error("expected server failed signature check")
	}
	if crypto3.signCalled {
		t.Error("Sign must not be called when Verify fails")
	}
	if io3.wroteSignature {
		t.Error("signature must not be written when Verify fails")
	}

	// 4) write signature error
	io4 := &ClientHandshakeFakeIO{writeSigErr: errors.New("wfail")}
	crypto4 := &ClientHandshakeFakeCrypto{verifyOK: true, signOut: []byte("client-sig")}
	ch4 := NewClientHandshake(nil, io4, crypto4)
	if err := ch4.SendSignature(edPub, edPriv, sessPub, hello, salt); err == nil {
		t.Error("expected write signature error")
	}

	// 5) success + precise payload checks (Verify and Sign concatenation order)
	io5 := &ClientHandshakeFakeIO{}
	cap5 := &CapturingCrypto{
		verifyOK: true,
		signOut:  []byte("client-sig-OK"),
	}
	ch5 := NewClientHandshake(nil, io5, cap5)
	if err := ch5.SendSignature(edPub, edPriv, sessPub, hello, salt); err != nil {
		t.Fatalf("SendSignature failed: %v", err)
	}
	if !io5.wroteSignature {
		t.Fatal("WriteClientSignature was not called")
	}
	if !bytes.Equal(io5.signatureArg.data, cap5.signOut) {
		t.Errorf("written signature = %x; want %x", io5.signatureArg.data, cap5.signOut)
	}

	// Verify payload must be: server.curvePublicKey || server.nonce || sessionSalt
	wantVerify := append(append([]byte{}, curvePub...), append(nonce, salt...)...)
	if !bytes.Equal(cap5.gotVerifyMsg, wantVerify) {
		t.Errorf("verify payload mismatch:\n got %x\nwant %x", cap5.gotVerifyMsg, wantVerify)
	}
	// Sign payload must be: client.sessionPublicKey || sessionSalt || server.nonce
	wantSign := append(append([]byte{}, sessPub...), append(salt, nonce...)...)
	if !bytes.Equal(cap5.gotSignMsg, wantSign) {
		t.Errorf("sign payload mismatch:\n got %x\nwant %x", cap5.gotSignMsg, wantSign)
	}
}
