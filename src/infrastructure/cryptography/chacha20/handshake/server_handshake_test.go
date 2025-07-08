package handshake

import (
	"bytes"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
	"net"
	"testing"
)

// fakeConn implements application.ConnectionAdapter
type fakeConn struct {
	in  *bytes.Buffer
	out *bytes.Buffer
}

func newFakeConn(input []byte) *fakeConn {
	var inBuf *bytes.Buffer
	if input != nil {
		inBuf = bytes.NewBuffer(input)
	}
	return &fakeConn{in: inBuf, out: &bytes.Buffer{}}
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

func (f *fakeConn) Close() error { return nil }

// stubCrypto satisfies Crypto for Sign/Verify
type stubCrypto struct {
	signature []byte
	verifyOK  bool
}

func (s *stubCrypto) GenerateEd25519KeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	panic("not implemented")
}

func (s *stubCrypto) GenerateX25519KeyPair() ([]byte, [32]byte, error) {
	panic("not implemented")
}

func (s *stubCrypto) GenerateRandomBytesArray(_ int) []byte {
	panic("not implemented")
}

func (s *stubCrypto) GenerateChaCha20KeysServerside(_, _ []byte, _ Hello) (sessionId [32]byte, clientToServerKey, serverToClientKey []byte, err error) {
	panic("not implemented")
}

func (s *stubCrypto) GenerateChaCha20KeysClientside(_, _ []byte, _ Hello) ([]byte, []byte, [32]byte, error) {
	panic("not implemented")
}

func (s *stubCrypto) Sign(_ ed25519.PrivateKey, _ []byte) []byte {
	return s.signature
}

func (s *stubCrypto) Verify(_ ed25519.PublicKey, _, _ []byte) bool {
	return s.verifyOK
}

// Build a minimal valid ClientHello buffer
func buildHello(t *testing.T) []byte {
	t.Helper()
	edPub, _, _ := ed25519.GenerateKey(rand.Reader)
	ch := NewClientHello(4, net.ParseIP("1.2.3.4"), edPub, make([]byte, curvePublicKeyLength), make([]byte, nonceLength))
	buf, err := ch.MarshalBinary()
	if err != nil {
		t.Fatalf("buildHello.MarshalBinary: %v", err)
	}
	return buf
}

func buildObfsHello(t *testing.T, key []byte) []byte {
	t.Helper()
	edPub, _, _ := ed25519.GenerateKey(rand.Reader)
	ch := NewClientHello(4, net.ParseIP("1.2.3.4"), edPub, make([]byte, curvePublicKeyLength), make([]byte, nonceLength))
	payload, _ := ch.MarshalBinary()
	pad := byte(5)
	padding := bytes.Repeat([]byte{0}, int(pad))
	mac := hmac.New(sha256.New, key)
	mac.Write(padding)
	mac.Write(payload)
	digest := mac.Sum(nil)
	buf := append([]byte{obfsHelloType, pad}, padding...)
	buf = append(buf, payload...)
	buf = append(buf, digest...)
	return buf
}

func TestReceiveClientHello_Success(t *testing.T) {
	buf := buildHello(t)
	conn := newFakeConn(buf)
	hs := NewServerHandshake(conn, nil)

	ch, err := hs.ReceiveClientHello()
	if err != nil {
		t.Fatalf("ReceiveClientHello error: %v", err)
	}
	if ch.ipVersion != 4 || !bytes.Equal(ch.ipAddress, net.ParseIP("1.2.3.4")) {
		t.Errorf("unexpected clientHello: %+v", ch)
	}
}

func TestReceiveClientHello_ReadError(t *testing.T) {
	conn := &fakeConn{in: nil, out: &bytes.Buffer{}}
	hs := NewServerHandshake(conn, nil)

	_, err := hs.ReceiveClientHello()
	if err == nil {
		t.Fatal("expected read error, got nil")
	}
}

func TestReceiveClientHello_UnmarshalError(t *testing.T) {
	conn := newFakeConn([]byte{0, 1, 2})
	hs := NewServerHandshake(conn, nil)

	_, err := hs.ReceiveClientHello()
	if err == nil {
		t.Fatal("expected unmarshal error, got nil")
	}
}

func TestReceiveClientHello_Obfuscated(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	buf := buildObfsHello(t, key)
	conn := newFakeConn(buf)
	hs := NewServerHandshake(conn, key)

	ch, err := hs.ReceiveClientHello()
	if err != nil {
		t.Fatalf("ReceiveClientHello error: %v", err)
	}
	if ch.ipVersion != 4 {
		t.Errorf("unexpected ipVersion %d", ch.ipVersion)
	}
}

func TestReceiveClientHello_ObfuscatedBadMAC(t *testing.T) {
	key := bytes.Repeat([]byte{2}, 32)
	buf := buildObfsHello(t, key)
	// use different key for server
	conn := newFakeConn(buf)
	hs := NewServerHandshake(conn, bytes.Repeat([]byte{3}, 32))
	if _, err := hs.ReceiveClientHello(); err == nil {
		t.Fatal("expected hmac error")
	}
}

func TestSendServerHello_Success(t *testing.T) {
	// prepare stubCrypto to return exactly 64‑byte signature
	sig := bytes.Repeat([]byte{0xAB}, signatureLength)
	c := &stubCrypto{signature: sig}

	curvePub := make([]byte, curvePublicKeyLength)
	_, _ = rand.Read(curvePub)
	nonce := make([]byte, nonceLength)
	_, _ = rand.Read(nonce)
	clientNonce := make([]byte, nonceLength)
	_, _ = rand.Read(clientNonce)

	conn := newFakeConn(nil)
	hs := NewServerHandshake(conn, nil)

	err := hs.SendServerHello(c, nil, nonce, curvePub, clientNonce)
	if err != nil {
		t.Fatalf("SendServerHello error: %v", err)
	}

	// verify what was written
	out := conn.out.Bytes()
	var sh ServerHello
	if err := sh.UnmarshalBinary(out); err != nil {
		t.Fatalf("UnmarshalBinary failed: %v", err)
	}
	if !bytes.Equal(sh.signature, sig) {
		t.Errorf("Signature = %x; want %x", sh.signature, sig)
	}
	if !bytes.Equal(sh.Nonce(), nonce) {
		t.Errorf("Nonce = %x; want %x", sh.Nonce(), nonce)
	}
	if !bytes.Equal(sh.CurvePublicKey(), curvePub) {
		t.Errorf("CurvePublicKey = %x; want %x", sh.CurvePublicKey(), curvePub)
	}
}

func TestSendServerHello_MarshalError(t *testing.T) {
	// make stubCrypto return wrong‑size signature
	sig := bytes.Repeat([]byte{0x00}, signatureLength-1)
	c := &stubCrypto{signature: sig}

	conn := newFakeConn(nil)
	hs := NewServerHandshake(conn, nil)

	err := hs.SendServerHello(c, nil, make([]byte, nonceLength), make([]byte, curvePublicKeyLength), make([]byte, nonceLength))
	if err == nil {
		t.Fatal("expected MarshalBinary error, got nil")
	}
}

func TestVerifyClientSignature_Success(t *testing.T) {
	// sign correct data so Verify logic would pass
	edPub, edPriv, _ := ed25519.GenerateKey(rand.Reader)
	hello := ClientHello{
		ipVersion:      4,
		ipAddress:      net.ParseIP("1.2.3.4"),
		edPublicKey:    edPub,
		curvePublicKey: make([]byte, curvePublicKeyLength),
		nonce:          make([]byte, nonceLength),
	}
	_, _ = rand.Read(hello.curvePublicKey)
	_, _ = rand.Read(hello.Nonce())

	serverNonce := make([]byte, nonceLength)
	_, _ = rand.Read(serverNonce)

	// compute a real Ed25519 signature over the concatenation
	data := append(append(hello.curvePublicKey, hello.Nonce()...), serverNonce...)
	sig := ed25519.Sign(edPriv, data)

	// feed that signature into fakeConn
	conn := newFakeConn(sig)
	// stubCrypto.Verify not used because we’re using real ed25519.Verify
	c := &stubCrypto{verifyOK: true}

	hs := NewServerHandshake(conn, nil)
	if err := hs.VerifyClientSignature(c, hello, serverNonce); err != nil {
		t.Fatalf("VerifyClientSignature error: %v", err)
	}
}

func TestVerifyClientSignature_ReadError(t *testing.T) {
	conn := &fakeConn{in: nil, out: &bytes.Buffer{}}
	hs := NewServerHandshake(conn, nil)
	err := hs.VerifyClientSignature(&stubCrypto{verifyOK: true}, ClientHello{}, nil)
	if err == nil {
		t.Fatal("expected read error, got nil")
	}
}

func TestVerifyClientSignature_UnmarshalError(t *testing.T) {
	// supply fewer than 64 bytes
	conn := newFakeConn([]byte{1, 2, 3})
	hs := NewServerHandshake(conn, nil)
	err := hs.VerifyClientSignature(&stubCrypto{verifyOK: true}, ClientHello{}, nil)
	if err == nil {
		t.Fatal("expected unmarshal error, got nil")
	}
}

func TestVerifyClientSignature_VerifyFail(t *testing.T) {
	// supply correct‑length data but stubCrypto.Verify=false
	conn := newFakeConn(bytes.Repeat([]byte{0xFF}, signatureLength))
	hs := NewServerHandshake(conn, nil)
	err := hs.VerifyClientSignature(&stubCrypto{verifyOK: false}, ClientHello{}, nil)
	if err == nil {
		t.Fatal("expected verification failure, got nil")
	}
}
