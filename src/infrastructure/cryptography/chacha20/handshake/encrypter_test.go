package handshake

import (
	"bytes"
	"crypto/rand"
	"testing"

	"golang.org/x/crypto/nacl/box"
)

func TestEncrypter_RoundTrip(t *testing.T) {
	pub, priv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	sender := NewEncrypter(pub, nil)
	receiver := NewEncrypter(pub, priv)
	msg := []byte("secret")
	enc, err := sender.Encrypt(msg)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	dec, ok, err := receiver.Decrypt(enc)
	if err != nil || !ok {
		t.Fatalf("decrypt failed: %v", err)
	}
	if !bytes.Equal(dec, msg) {
		t.Errorf("roundtrip mismatch")
	}
}

func TestEncrypter_Plain(t *testing.T) {
	e := NewEncrypter(nil, nil)
	msg := []byte("plain")
	out, err := e.Encrypt(msg)
	if err != nil {
		t.Fatalf("encrypt plain: %v", err)
	}
	if !bytes.Equal(out, msg) {
		t.Errorf("data changed")
	}
	dec, ok, err := e.Decrypt(out)
	if err != nil || ok {
		t.Fatalf("decrypt plain unexpected: %v", err)
	}
	if !bytes.Equal(dec, msg) {
		t.Errorf("decrypt mismatch")
	}
}
