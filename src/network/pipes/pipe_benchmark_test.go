package pipes

import (
	"bytes"
	"errors"
	"testing"
)

type fakeSession struct{}

func (fs *fakeSession) Encrypt(data []byte) ([]byte, error) {
	return append([]byte("enc:"), data...), nil
}

func (fs *fakeSession) Decrypt(data []byte) ([]byte, error) {
	prefix := []byte("enc:")
	if len(data) < len(prefix) || string(data[:len(prefix)]) != string(prefix) {
		return nil, errors.New("invalid data")
	}
	return data[len(prefix):], nil
}

func BenchmarkDefaultPipe(b *testing.B) {
	data := []byte("test data for benchmarking")
	from := bytes.NewBuffer(data)
	to := new(bytes.Buffer)
	pipe := NewDefaultPipe(from, to)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := pipe.Pass(data)
		if err != nil {
			b.Fatal(err)
		}
		to.Reset()
	}
}

func BenchmarkEncryptionPipe(b *testing.B) {
	data := []byte("test data for benchmarking")
	from := bytes.NewBuffer(data)
	to := new(bytes.Buffer)
	defaultPipe := NewDefaultPipe(from, to)
	session := &fakeSession{}

	encPipe := NewEncryptionPipe(defaultPipe, session)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := encPipe.Pass(data)
		if err != nil {
			b.Fatal(err)
		}
		to.Reset()
	}
}

func BenchmarkDecryptionPipe(b *testing.B) {
	data := []byte("test data for benchmarking")
	session := &fakeSession{}
	encryptedData, err := session.Encrypt(data)
	if err != nil {
		b.Fatal(err)
	}

	from := bytes.NewBuffer(encryptedData)
	to := new(bytes.Buffer)
	defaultPipe := NewDefaultPipe(from, to)

	decPipe := NewDecryptionPipe(defaultPipe, session)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := decPipe.Pass(encryptedData)
		if err != nil {
			b.Fatal(err)
		}
		to.Reset()
	}
}
