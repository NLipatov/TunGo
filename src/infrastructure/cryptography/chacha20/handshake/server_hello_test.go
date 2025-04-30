package handshake

import (
	"bytes"
	"errors"
	"testing"
)

// helper to build valid fields
func validSlices() (sig, nonce, curve []byte) {
	sig = make([]byte, signatureLength)
	nonce = make([]byte, nonceLength)
	curve = make([]byte, curvePublicKeyLength)
	for i := range sig {
		sig[i] = byte(i)
	}
	for i := range nonce {
		nonce[i] = byte(i + 10)
	}
	for i := range curve {
		curve[i] = byte(i + 20)
	}
	return
}

func TestMarshalBinary_Success(t *testing.T) {
	sig, nonce, curve := validSlices()
	s := &ServerHello{Signature: sig, Nonce: nonce, CurvePublicKey: curve}
	buf, err := s.MarshalBinary()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// reconstruct and compare
	var s2 ServerHello
	if err := s2.UnmarshalBinary(buf); err != nil {
		t.Fatalf("roundtrip UnmarshalBinary failed: %v", err)
	}
	if !bytes.Equal(s2.Signature, sig) {
		t.Errorf("Signature: got %v, want %v", s2.Signature, sig)
	}
	if !bytes.Equal(s2.Nonce, nonce) {
		t.Errorf("Nonce: got %v, want %v", s2.Nonce, nonce)
	}
	if !bytes.Equal(s2.CurvePublicKey, curve) {
		t.Errorf("CurvePublicKey: got %v, want %v", s2.CurvePublicKey, curve)
	}
}

func TestMarshalBinary_Errors(t *testing.T) {
	sig, nonce, curve := validSlices()

	cases := []struct {
		srv   *ServerHello
		exErr error
		name  string
	}{
		{&ServerHello{Signature: sig[:1], Nonce: nonce, CurvePublicKey: curve}, ErrInvalidSignatureLength, "bad-signature"},
		{&ServerHello{Signature: sig, Nonce: nonce[:1], CurvePublicKey: curve}, ErrInvalidNonceLength, "bad-nonce"},
		{&ServerHello{Signature: sig, Nonce: nonce, CurvePublicKey: curve[:1]}, ErrInvalidCurveKeyLength, "bad-curve"},
	}
	for _, c := range cases {
		if _, err := c.srv.MarshalBinary(); !errors.Is(err, c.exErr) {
			t.Errorf("%s: expected %v, got %v", c.name, c.exErr, err)
		}
	}
}

func TestUnmarshalBinary_ErrData(t *testing.T) {
	var s ServerHello
	short := make([]byte, signatureLength+nonceLength+curvePublicKeyLength-1)
	err := s.UnmarshalBinary(short)
	if !errors.Is(err, ErrInvalidData) {
		t.Errorf("expected ErrInvalidData, got %v", err)
	}
}

func TestNewServerHello(t *testing.T) {
	sig, nonce, curve := validSlices()
	// success
	_, err := NewServerHello(sig, nonce, curve)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// error wrap
	_, err = NewServerHello(sig[:1], nonce, curve)
	if err == nil || !errors.Is(err, ErrInvalidSignatureLength) {
		t.Errorf("expected ErrInvalidSignatureLength, got %v", err)
	}
}
