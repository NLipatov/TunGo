package handshake

import (
	"bytes"
	"testing"
)

func TestDefaultSessionIdDeriver_Derive_Deterministic(t *testing.T) {
	secret := []byte("this is a shared secret")
	salt := []byte("this is a salt value!!!") // any length

	d1 := NewDefaultSessionIdDeriver(secret, salt)
	id1, err := d1.Derive()
	if err != nil {
		t.Fatalf("Derive() unexpected error: %v", err)
	}

	d2 := NewDefaultSessionIdDeriver(secret, salt)
	id2, err := d2.Derive()
	if err != nil {
		t.Fatalf("Derive() unexpected error: %v", err)
	}

	if !bytes.Equal(id1[:], id2[:]) {
		t.Errorf("Derive() not deterministic; got\n%x\nand\n%x\nfor same inputs", id1, id2)
	}
}

func TestDefaultSessionIdDeriver_Derive_VariesWithInputs(t *testing.T) {
	secret1 := []byte("secret-one")
	secret2 := []byte("secret-two")
	salt := []byte("same-salt-constant-for-test")

	idA, err := NewDefaultSessionIdDeriver(secret1, salt).Derive()
	if err != nil {
		t.Fatalf("Derive() error for secret1: %v", err)
	}
	idB, err := NewDefaultSessionIdDeriver(secret2, salt).Derive()
	if err != nil {
		t.Fatalf("Derive() error for secret2: %v", err)
	}
	if bytes.Equal(idA[:], idB[:]) {
		t.Errorf("Derive() should differ for different secrets, but got same %x", idA)
	}

	idC, err := NewDefaultSessionIdDeriver(secret1, []byte("other-salt")).Derive()
	if err != nil {
		t.Fatalf("Derive() error for different salt: %v", err)
	}
	if bytes.Equal(idA[:], idC[:]) {
		t.Errorf("Derive() should differ for different salts, but got same %x", idA)
	}
}
