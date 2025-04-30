package handshake

import (
	"bytes"
	"testing"
)

func TestClientCryptoMarshalBinary_Success(t *testing.T) {
	ex := make([]byte, signatureLength)
	for i := range ex {
		ex[i] = byte(i)
	}
	cs := &ClientSignature{Signature: ex}
	got, err := cs.MarshalBinary()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(got, ex) {
		t.Errorf("MarshalBinary = %v; want %v", got, ex)
	}
}

func TestClientCryptoMarshalBinary_ErrorLength(t *testing.T) {
	ex := make([]byte, signatureLength-1)
	cs := &ClientSignature{Signature: ex}
	_, err := cs.MarshalBinary()
	if err != ErrInvalidClientSignatureLength {
		t.Errorf("expected ErrInvalidClientSignatureLength, got %v", err)
	}
}

func TestClientCryptoUnmarshalBinary_Success(t *testing.T) {
	ex := make([]byte, signatureLength)
	for i := range ex {
		ex[i] = byte(i + 5)
	}
	cs := &ClientSignature{}
	err := cs.UnmarshalBinary(ex)
	if err != nil {
		t.Fatalf("unexpected UnmarshalBinary error: %v", err)
	}
	if !bytes.Equal(cs.Signature, ex) {
		t.Errorf("Signature = %v; want %v", cs.Signature, ex)
	}
}

func TestClientCryptoUnmarshalBinary_ErrorLength(t *testing.T) {
	ex := make([]byte, signatureLength-1)
	cs := &ClientSignature{}
	err := cs.UnmarshalBinary(ex)
	if err != ErrInvalidClientSignatureLength {
		t.Errorf("expected ErrInvalidClientSignatureLength, got %v", err)
	}
}

func TestClientCryptoNewClientSignature_Success(t *testing.T) {
	ex := make([]byte, signatureLength)
	cs, err := NewClientSignature(ex)
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}
	if !bytes.Equal(cs.Signature, ex) {
		t.Errorf("Signature = %v; want %v", cs.Signature, ex)
	}
}

func TestClientCryptoNewClientSignature_Error(t *testing.T) {
	ex := make([]byte, signatureLength+1)
	_, err := NewClientSignature(ex)
	if err == nil {
		t.Error("expected error for invalid length, got nil")
	}
}
