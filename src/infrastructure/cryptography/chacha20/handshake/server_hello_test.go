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

	if !bytes.Equal(sH.Signature, data[:signatureLength]) {
		t.Errorf("Signature does not match. Expected %v, got %v", data[:signatureLength], sH.Signature)
	}
	if !bytes.Equal(sH.Nonce, data[signatureLength:signatureLength+nonceLength]) {
		t.Errorf("Nonce does not match. Expected %v, got %v", data[signatureLength:signatureLength+nonceLength], sH.Nonce)
	}
	if !bytes.Equal(sH.CurvePublicKey, data[signatureLength+nonceLength:]) {
		t.Errorf("CurvePublicKey does not match. Expected %v, got %v", data[signatureLength+nonceLength:], sH.CurvePublicKey)
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

	sH := &ServerHello{Signature: sig, Nonce: nonce, CurvePublicKey: curvePub}
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
	sH := &ServerHello{Signature: make([]byte, signatureLength-1), Nonce: make([]byte, nonceLength), CurvePublicKey: make([]byte, curvePublicKeyLength)}
	if _, err := sH.MarshalBinary(); err == nil {
		t.Fatal("Expected error for invalid signature length, got nil")
	}
}

func TestServerHello_MarshalBinary_InvalidNonce(t *testing.T) {
	// nonce wrong length
	sH := &ServerHello{Signature: make([]byte, signatureLength), Nonce: make([]byte, nonceLength-1), CurvePublicKey: make([]byte, curvePublicKeyLength)}
	if _, err := sH.MarshalBinary(); err == nil {
		t.Fatal("Expected error for invalid nonce length, got nil")
	}
}

func TestServerHello_MarshalBinary_InvalidCurvePublicKey(t *testing.T) {
	// curve key wrong length
	sH := &ServerHello{Signature: make([]byte, signatureLength), Nonce: make([]byte, nonceLength), CurvePublicKey: make([]byte, curvePublicKeyLength-1)}
	if _, err := sH.MarshalBinary(); err == nil {
		t.Fatal("Expected error for invalid curve public key length, got nil")
	}
}
