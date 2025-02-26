package pipes

import (
	"bytes"
	"errors"
	"testing"
)

type DecryptionPipeFakeSession struct{}

func (fs *DecryptionPipeFakeSession) Encrypt(data []byte) ([]byte, error) {
	return append([]byte("enc:"), data...), nil
}

func (fs *DecryptionPipeFakeSession) Decrypt(data []byte) ([]byte, error) {
	prefix := []byte("enc:")
	if len(data) < len(prefix) || string(data[:len(prefix)]) != string(prefix) {
		return nil, errors.New("invalid encrypted data")
	}
	return data[len(prefix):], nil
}

type ErrorSession struct{}

func (es *ErrorSession) Encrypt(_ []byte) ([]byte, error) {
	return nil, nil
}

func (es *ErrorSession) Decrypt(_ []byte) ([]byte, error) {
	return nil, errors.New("decryption failed")
}

func TestDecryptionPipe_Success(t *testing.T) {
	encryptedData := []byte("enc:hello")
	expectedDecrypted := "hello"

	var buf bytes.Buffer
	defaultPipe := NewDefaultPipe(nil, &buf)

	dp := NewDecryptionPipe(defaultPipe, &DecryptionPipeFakeSession{})

	if err := dp.Pass(encryptedData); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if buf.String() != expectedDecrypted {
		t.Errorf("expected %q, got %q", expectedDecrypted, buf.String())
	}
}

func TestDecryptionPipe_Error(t *testing.T) {
	invalidData := []byte("invalid data")

	var buf bytes.Buffer
	defaultPipe := NewDefaultPipe(nil, &buf)

	dp := NewDecryptionPipe(defaultPipe, &ErrorSession{})

	err := dp.Pass(invalidData)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "decryption failed" {
		t.Errorf("expected error 'decryption failed', got %q", err.Error())
	}
}
