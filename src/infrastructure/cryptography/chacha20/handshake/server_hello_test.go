package handshake

import (
	"bytes"
	"testing"
)

func TestServerHello_UnmarshalBinary_Success(t *testing.T) {
	sig := bytes.Repeat([]byte{0x01}, signatureLength)
	nonce := bytes.Repeat([]byte{0x02}, nonceLength)
	curve := bytes.Repeat([]byte{0x03}, curvePublicKeyLength)

	orig := NewServerHello(sig, nonce, curve)
	data, err := orig.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var got ServerHello
	if err := got.UnmarshalBinary(data); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if !bytes.Equal(got.signature, sig) {
		t.Errorf("signature mismatch")
	}
	if !bytes.Equal(got.nonce, nonce) {
		t.Errorf("nonce mismatch")
	}
	if !bytes.Equal(got.curvePublicKey, curve) {
		t.Errorf("curvePublicKey mismatch")
	}
}

func TestServerHello_UnmarshalBinary_InvalidLength(t *testing.T) {
	data := make([]byte, signatureLength+nonceLength+10)

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

	var parsed ServerHello
	if err := parsed.UnmarshalBinary(buf); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if !bytes.Equal(parsed.signature, sig) || !bytes.Equal(parsed.nonce, nonce) || !bytes.Equal(parsed.curvePublicKey, curvePub) {
		t.Errorf("fields mismatch after roundtrip")
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
