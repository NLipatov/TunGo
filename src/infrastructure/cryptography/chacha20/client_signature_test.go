package chacha20

import (
	"bytes"
	"testing"
)

func TestClientSignature_WriteAndRead(t *testing.T) {
	signature := make([]byte, 64)
	for i := 0; i < 64; i++ {
		signature[i] = byte(i)
	}

	clientSignature := &ClientSignature{}

	writtenData, err := clientSignature.Write(&signature)
	if err != nil {
		t.Fatalf("unexpected error during Write: %v", err)
	}

	if !bytes.Equal(*writtenData, signature) {
		t.Errorf("written signature mismatch: expected %v, got %v", signature, *writtenData)
	}

	readSignature, err := clientSignature.Read(*writtenData)
	if err != nil {
		t.Fatalf("unexpected error during Read: %v", err)
	}

	if !bytes.Equal(readSignature.ClientSignature, signature) {
		t.Errorf("read signature mismatch: expected %v, got %v", signature, readSignature.ClientSignature)
	}
}

func TestClientSignature_InvalidInput(t *testing.T) {
	clientSignature := &ClientSignature{}

	invalidSignature := make([]byte, 63) // Меньше 64 байт
	_, err := clientSignature.Write(&invalidSignature)
	if err == nil {
		t.Error("expected error for invalid signature length, got nil")
	}

	invalidData := make([]byte, 63) // Меньше 64 байт
	_, err = clientSignature.Read(invalidData)
	if err == nil {
		t.Error("expected error for invalid data length, got nil")
	}
}
