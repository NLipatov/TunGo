package handshake

import (
	"crypto/ed25519"
	"errors"
	"testing"
)

type serverHandshakeTestSendErrMockIO struct{}

func (e *serverHandshakeTestSendErrMockIO) ReceiveClientHello() (ClientHello, error) {
	return ClientHello{}, nil
}
func (e *serverHandshakeTestSendErrMockIO) SendServerHello(_ ServerHello) error {
	return errors.New("forced send error")
}
func (e *serverHandshakeTestSendErrMockIO) ReadClientSignature() (Signature, error) {
	return Signature{}, nil
}

type serverHandshakeTestMockIO struct {
	hello        ClientHello
	helloErr     error
	serverHello  ServerHello
	serverErr    error
	signature    Signature
	signErr      error
	writeHistory []ServerHello
}

func (m *serverHandshakeTestMockIO) ReceiveClientHello() (ClientHello, error) {
	return m.hello, m.helloErr
}

func (m *serverHandshakeTestMockIO) SendServerHello(hello ServerHello) error {
	m.writeHistory = append(m.writeHistory, hello)
	return m.serverErr
}

func (m *serverHandshakeTestMockIO) ReadClientSignature() (Signature, error) {
	return m.signature, m.signErr
}

type mockCrypto struct {
	signature []byte
	verifyOK  bool
}

func (m *mockCrypto) Sign(_ ed25519.PrivateKey, _ []byte) []byte { return m.signature }
func (m *mockCrypto) Verify(_ ed25519.PublicKey, _ []byte, _ []byte) bool {
	return m.verifyOK
}
func (m *mockCrypto) GenerateEd25519KeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	panic("not used")
}
func (m *mockCrypto) GenerateX25519KeyPair() ([]byte, [32]byte, error) { panic("not used") }
func (m *mockCrypto) GenerateRandomBytesArray(_ int) []byte            { panic("not used") }
func (m *mockCrypto) GenerateChaCha20KeysServerside(_, _ []byte, _ Hello) ([32]byte, []byte, []byte, error) {
	panic("not used")
}
func (m *mockCrypto) GenerateChaCha20KeysClientside(_, _ []byte, _ Hello) ([]byte, []byte, [32]byte, error) {
	panic("not used")
}

func minimalClientHello() ClientHello {
	pub, _, _ := ed25519.GenerateKey(nil)
	return NewClientHello(4, []byte{127, 0, 0, 1}, pub, make([]byte, curvePublicKeyLength), make([]byte, nonceLength))
}
func minimalSignature() Signature {
	return NewSignature(make([]byte, signatureLength))
}

func TestServerHandshake_ReceiveClientHello_Success(t *testing.T) {
	io := &serverHandshakeTestMockIO{hello: minimalClientHello()}
	hs := NewServerHandshake(io)
	got, err := hs.ReceiveClientHello()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ipVersion != 4 {
		t.Error("wrong ipVersion")
	}
}

func TestServerHandshake_ReceiveClientHello_Error(t *testing.T) {
	io := &serverHandshakeTestMockIO{helloErr: errors.New("fail")}
	hs := NewServerHandshake(io)
	_, err := hs.ReceiveClientHello()
	if err == nil {
		t.Error("expected error")
	}
}

func TestServerHandshake_SendServerHello_Success(t *testing.T) {
	io := &serverHandshakeTestMockIO{}
	hs := NewServerHandshake(io)
	crypto := &mockCrypto{signature: make([]byte, signatureLength)}
	err := hs.SendServerHello(
		crypto, nil, make([]byte, nonceLength), make([]byte, curvePublicKeyLength), make([]byte, nonceLength))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(io.writeHistory) != 1 {
		t.Error("server hello not written")
	}
}

func TestServerHandshake_SendServerHello_Error(t *testing.T) {
	io := &serverHandshakeTestMockIO{serverErr: errors.New("send fail")}
	hs := NewServerHandshake(io)
	crypto := &mockCrypto{signature: make([]byte, signatureLength)}
	err := hs.SendServerHello(
		crypto, nil, make([]byte, nonceLength), make([]byte, curvePublicKeyLength), make([]byte, nonceLength))
	if err == nil {
		t.Error("expected error from SendServerHello")
	}
}

func TestServerHandshake_SendServerHello_InvalidSignature(t *testing.T) {
	io := &serverHandshakeTestSendErrMockIO{}
	hs := NewServerHandshake(io)
	crypto := &mockCrypto{signature: make([]byte, signatureLength-1)}
	err := hs.SendServerHello(
		crypto, nil, make([]byte, nonceLength), make([]byte, curvePublicKeyLength), make([]byte, nonceLength))
	if err == nil {
		t.Error("expected MarshalBinary error, got nil")
	}
}

func TestServerHandshake_VerifyClientSignature_Success(t *testing.T) {
	io := &serverHandshakeTestMockIO{signature: minimalSignature()}
	hs := NewServerHandshake(io)
	crypto := &mockCrypto{verifyOK: true}
	err := hs.VerifyClientSignature(crypto, minimalClientHello(), make([]byte, nonceLength))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServerHandshake_VerifyClientSignature_ReadError(t *testing.T) {
	io := &serverHandshakeTestMockIO{signErr: errors.New("no sig")}
	hs := NewServerHandshake(io)
	crypto := &mockCrypto{verifyOK: true}
	err := hs.VerifyClientSignature(crypto, minimalClientHello(), make([]byte, nonceLength))
	if err == nil {
		t.Error("expected error")
	}
}

func TestServerHandshake_VerifyClientSignature_VerifyFail(t *testing.T) {
	io := &serverHandshakeTestMockIO{signature: minimalSignature()}
	hs := NewServerHandshake(io)
	crypto := &mockCrypto{verifyOK: false}
	err := hs.VerifyClientSignature(crypto, minimalClientHello(), make([]byte, nonceLength))
	if err == nil {
		t.Error("expected verification fail")
	}
}
