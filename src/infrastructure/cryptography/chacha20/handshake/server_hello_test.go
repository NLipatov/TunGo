package handshake

import (
	"bytes"
	"testing"
)

func TestServerHello_UnmarshalBinary_Success(t *testing.T) {
	data := make([]byte, signatureLength+nonceLength+curvePublicKeyLength)
	copy(data[:signatureLength], bytes.Repeat([]byte{0x01}, signatureLength))
	copy(data[signatureLength:signatureLength+nonceLength], bytes.Repeat([]byte{0x02}, nonceLength))
	copy(data[signatureLength+nonceLength:], bytes.Repeat([]byte{0x03}, curvePublicKeyLength))

	sH := &ServerHello{}
	err := sH.UnmarshalBinary(data)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if !bytes.Equal(sH.signature, data[:signatureLength]) {
		t.Errorf("signature does not match. Expected %v, got %v", data[:signatureLength], sH.signature)
	}
	if !bytes.Equal(sH.nonce, data[signatureLength:signatureLength+nonceLength]) {
		t.Errorf("nonce does not match. Expected %v, got %v", data[signatureLength:signatureLength+nonceLength], sH.nonce)
	}
	if !bytes.Equal(sH.curvePublicKey, data[signatureLength+nonceLength:]) {
		t.Errorf("curvePublicKey does not match. Expected %v, got %v", data[signatureLength+nonceLength:], sH.curvePublicKey)
	}
}

func TestServerHello_UnmarshalBinary_InvalidLength(t *testing.T) {
	data := make([]byte, signatureLength+nonceLength+10) // wrong length

	sH := &ServerHello{}
	err := sH.UnmarshalBinary(data)
	if err == nil {
		t.Fatalf("Expected error for invalid data length, got nil")
	}
}

func TestServerHello_MarshalBinary_Success(t *testing.T) {
	sig := bytes.Repeat([]byte{0x01}, signatureLength)
	nonce := bytes.Repeat([]byte{0x02}, nonceLength)
	curvePub := bytes.Repeat([]byte{0x03}, curvePublicKeyLength)

	sH := &ServerHello{signature: sig, nonce: nonce, curvePublicKey: curvePub}
	buf, err := sH.MarshalBinary()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	expected := append(append(sig, nonce...), curvePub...)
	if !bytes.Equal(buf, expected) {
		t.Errorf("Result does not match. Expected %v, got %v", expected, buf)
	}
}

func TestServerHello_MarshalBinary_InvalidSignature(t *testing.T) {
	// signature wrong length
	sH := &ServerHello{signature: make([]byte, signatureLength-1), nonce: make([]byte, nonceLength), curvePublicKey: make([]byte, curvePublicKeyLength)}
	if _, err := sH.MarshalBinary(); err == nil {
		t.Fatal("Expected error for invalid signature length, got nil")
	}
}

func TestServerHello_MarshalBinary_InvalidNonce(t *testing.T) {
	// nonce wrong length
	sH := &ServerHello{signature: make([]byte, signatureLength), nonce: make([]byte, nonceLength-1), curvePublicKey: make([]byte, curvePublicKeyLength)}
	if _, err := sH.MarshalBinary(); err == nil {
		t.Fatal("Expected error for invalid nonce length, got nil")
	}
}

func TestServerHello_MarshalBinary_InvalidCurvePublicKey(t *testing.T) {
	// curve key wrong length
	sH := &ServerHello{signature: make([]byte, signatureLength), nonce: make([]byte, nonceLength), curvePublicKey: make([]byte, curvePublicKeyLength-1)}
	if _, err := sH.MarshalBinary(); err == nil {
		t.Fatal("Expected error for invalid curve public key length, got nil")
	}
}
