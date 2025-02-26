package pipes

import (
	"bytes"
	"errors"
	"testing"
)

type EncryptionPipeErrorSession struct{}

func (es *EncryptionPipeErrorSession) Encrypt(_ []byte) ([]byte, error) {
	return nil, errors.New("encryption failed")
}

func (es *EncryptionPipeErrorSession) Decrypt(_ []byte) ([]byte, error) {
	return nil, nil
}

func TestEncryptionPipe_Success(t *testing.T) {
	data := []byte("hello")
	expected := "enc:hello"

	var buf bytes.Buffer
	defaultPipe := NewDefaultPipe(nil, &buf)

	ep := NewEncryptionPipe(defaultPipe, &DecryptionPipeFakeSession{})

	if err := ep.Pass(data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if buf.String() != expected {
		t.Errorf("expected output %q, got %q", expected, buf.String())
	}
}

func TestEncryptionPipe_Error(t *testing.T) {
	data := []byte("hello")

	var buf bytes.Buffer
	defaultPipe := NewDefaultPipe(nil, &buf)

	ep := NewEncryptionPipe(defaultPipe, &EncryptionPipeErrorSession{})

	err := ep.Pass(data)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "encryption failed" {
		t.Errorf("expected error 'encryption failed', got %q", err.Error())
	}
}
