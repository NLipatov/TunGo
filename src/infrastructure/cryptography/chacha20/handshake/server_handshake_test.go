package handshake

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"testing"
)

// fakeConn implements application.ConnectionAdapter
// with controllable Read, Write and Close.
type fakeConn struct {
	in  *bytes.Buffer
	out *bytes.Buffer
}

func newFakeConn(input []byte) *fakeConn {
	return &fakeConn{in: bytes.NewBuffer(input), out: &bytes.Buffer{}}
}

func (f *fakeConn) Read(p []byte) (int, error) {
	if f.in == nil {
		return 0, io.EOF
	}
	return f.in.Read(p)
}

func (f *fakeConn) Write(p []byte) (int, error) {
	if f.out == nil {
		return 0, errors.New("write closed")
	}
	return f.out.Write(p)
}

func (f *fakeConn) Close() error {
	return nil
}

// stubCrypto implements full Crypto interface
// for signing, verification and ChaCha20 key derivation.
type stubCrypto struct {
	signature []byte
	verifyOK  bool
}

func (s *stubCrypto) Sign(privateKey ed25519.PrivateKey, data []byte) []byte {
	return s.signature
}

func (s *stubCrypto) Verify(publicKey ed25519.PublicKey, data []byte, signature []byte) bool {
	return s.verifyOK
}

func (s *stubCrypto) GenerateEd25519KeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return nil, nil, nil
}

func (s *stubCrypto) GenerateX25519KeyPair() ([]byte, [32]byte, error) {
	return nil, [32]byte{}, nil
}

func (s *stubCrypto) GenerateRandomBytesArray(n int) []byte {
	return make([]byte, n)
}

// add missing ChaCha20 key derivation stub
func (s *stubCrypto) GenerateChaCha20KeysServerside(
	curvePrivate []byte,
	serverNonce []byte,
	hello ClientHello,
) (sessionId [32]byte, clientToServerKey []byte, serverToClientKey []byte, err error) {
	// return empty values for testing
	return [32]byte{}, nil, nil, nil
}

func TestReceiveClientHello_Success(t *testing.T) {
	edPub, _, _ := ed25519.GenerateKey(rand.Reader)
	curvePub := make([]byte, curvePublicKeyLength)
	rand.Read(curvePub)
	nonce := make([]byte, nonceLength)
	rand.Read(nonce)
	ch := NewClientHello(4, "10.0.0.5", edPub, curvePub, nonce)
	buf, err := ch.MarshalBinary()
	if err != nil {
		t.Fatalf("failed to marshal ClientHello: %v", err)
	}

	conn := newFakeConn(buf)
	hs := NewServerHandshake(conn)
	got, err := hs.ReceiveClientHello()
	if err != nil {
		t.Fatalf("ReceiveClientHello error: %v", err)
	}
	if got.ipVersion != ch.ipVersion {
		t.Errorf("ipVersion = %d; want %d", got.ipVersion, ch.ipVersion)
	}
	if got.ipAddress != ch.ipAddress {
		t.Errorf("ipAddress = %q; want %q", got.ipAddress, ch.ipAddress)
	}
}

func TestReceiveClientHello_ReadError(t *testing.T) {
	conn := &fakeConn{in: nil, out: &bytes.Buffer{}}
	hs := NewServerHandshake(conn)
	_, err := hs.ReceiveClientHello()
	if err == nil {
		t.Fatal("expected read error, got nil")
	}
}

func TestReceiveClientHello_UnmarshalError(t *testing.T) {
	conn := newFakeConn([]byte{0, 1, 2, 3})
	hs := NewServerHandshake(conn)
	_, err := hs.ReceiveClientHello()
	if err == nil {
		t.Fatal("expected unmarshal error, got nil")
	}
}

func TestSendServerHello_Success(t *testing.T) {
	sig := bytes.Repeat([]byte{0xAB}, signatureLength)
	c := &stubCrypto{signature: sig}
	curvePub := make([]byte, curvePublicKeyLength)
	rand.Read(curvePub)
	nonce := make([]byte, nonceLength)
	rand.Read(nonce)
	clientNonce := []byte("client-nonce-0123456789abcdef012345")[:nonceLength]

	conn := newFakeConn(nil)
	hs := NewServerHandshake(conn)
	err := hs.SendServerHello(c, nil, nonce, curvePub, clientNonce)
	if err != nil {
		t.Fatalf("SendServerHello error: %v", err)
	}
	out := conn.out.Bytes()
	var sh ServerHello
	err = sh.UnmarshalBinary(out)
	if err != nil {
		t.Fatalf("unmarshal of sent ServerHello failed: %v", err)
	}
	if !bytes.Equal(sh.Signature, sig) {
		t.Errorf("Signature = %v; want %v", sh.Signature, sig)
	}
}

func TestSendServerHello_MarshalError(t *testing.T) {
	sig := bytes.Repeat([]byte{0x01}, signatureLength-1)
	c := &stubCrypto{signature: sig}
	curvePub := make([]byte, curvePublicKeyLength)
	rand.Read(curvePub)
	nonce := make([]byte, nonceLength)
	rand.Read(nonce)
	clientNonce := make([]byte, nonceLength)
	rand.Read(clientNonce)

	conn := newFakeConn(nil)
	hs := NewServerHandshake(conn)
	err := hs.SendServerHello(c, nil, nonce, curvePub, clientNonce)
	if err == nil {
		t.Fatal("expected MarshalBinary error, got nil")
	}
}

func TestVerifyClientSignature_Success(t *testing.T) {
	edPub, edPriv, _ := ed25519.GenerateKey(rand.Reader)
	curvePub := make([]byte, curvePublicKeyLength)
	rand.Read(curvePub)
	nonce := make([]byte, nonceLength)
	rand.Read(nonce)
	hello := ClientHello{ipVersion: 4, ipAddress: "", edPublicKey: edPub, curvePublicKey: curvePub, clientNonce: nonce}
	serverNonce := []byte("server-nonce-0123456789abcdef0123456")[:nonceLength]
	data := append(append(curvePub, nonce...), serverNonce...)
	sig := ed25519.Sign(edPriv, data)

	conn := newFakeConn(sig)
	hs := NewServerHandshake(conn)
	c := &stubCrypto{verifyOK: true}
	err := hs.VerifyClientSignature(c, hello, serverNonce)
	if err != nil {
		t.Fatalf("VerifyClientSignature error: %v", err)
	}
}

func TestVerifyClientSignature_ReadError(t *testing.T) {
	conn := &fakeConn{in: nil, out: &bytes.Buffer{}}
	hs := NewServerHandshake(conn)
	c := &stubCrypto{verifyOK: true}
	err := hs.VerifyClientSignature(c, ClientHello{}, nil)
	if err == nil {
		t.Fatal("expected read error, got nil")
	}
}

func TestVerifyClientSignature_UnmarshalError(t *testing.T) {
	conn := newFakeConn([]byte{1, 2, 3})
	hs := NewServerHandshake(conn)
	c := &stubCrypto{verifyOK: true}
	err := hs.VerifyClientSignature(c, ClientHello{}, nil)
	if err == nil {
		t.Fatal("expected unmarshal error, got nil")
	}
}

func TestVerifyClientSignature_VerifyFail(t *testing.T) {
	sig := make([]byte, signatureLength)
	conn := newFakeConn(sig)
	hello := ClientHello{edPublicKey: nil, curvePublicKey: nil, clientNonce: nil}
	hs := NewServerHandshake(conn)
	c := &stubCrypto{verifyOK: false}
	err := hs.VerifyClientSignature(c, hello, []byte{0})
	if err == nil {
		t.Fatal("expected verification failure, got nil")
	}
}
