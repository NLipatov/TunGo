package chacha20

import (
	"encoding/binary"
	"errors"
	"testing"
)

func dummyNonceBytes(low uint64, high uint32) [12]byte {
	var b [12]byte
	binary.BigEndian.PutUint64(b[:8], low)
	binary.BigEndian.PutUint32(b[8:], high)
	return b
}

func TestInsertUnique(t *testing.T) {
	nb := NewNonceBuf(3)

	nonce1 := dummyNonceBytes(1, 2)
	if err := nb.Insert(nonce1); err != nil {
		t.Fatalf("unexpected error on Insert nonce1: %v", err)
	}

	nonce2 := dummyNonceBytes(3, 4)
	if err := nb.Insert(nonce2); err != nil {
		t.Fatalf("unexpected error on Insert nonce2: %v", err)
	}

	nonce3 := dummyNonceBytes(5, 6)
	if err := nb.Insert(nonce3); err != nil {
		t.Fatalf("unexpected error on Insert nonce3: %v", err)
	}
}

func TestInsertDuplicate(t *testing.T) {
	nb := NewNonceBuf(3)
	nonce := dummyNonceBytes(10, 20)
	if err := nb.Insert(nonce); err != nil {
		t.Fatalf("unexpected error on first Insert: %v", err)
	}
	err := nb.Insert(nonce)
	if err == nil {
		t.Fatalf("expected error on duplicate Insert, got nil")
	}
	if !errors.Is(err, ErrNonUniqueNonce) {
		t.Fatalf("expected ErrNonUniqueNonce, got %v", err)
	}
}

func TestBufferWrapAround(t *testing.T) {
	nb := NewNonceBuf(2)
	nonce1 := dummyNonceBytes(100, 200)
	nonce2 := dummyNonceBytes(101, 201)
	nonce3 := dummyNonceBytes(102, 202)

	if err := nb.Insert(nonce1); err != nil {
		t.Fatalf("Insert nonce1 error: %v", err)
	}
	if err := nb.Insert(nonce2); err != nil {
		t.Fatalf("Insert nonce2 error: %v", err)
	}
	if err := nb.Insert(nonce3); err != nil {
		t.Fatalf("Insert nonce3 error: %v", err)
	}
	if _, exists := nb.set[nonce1]; exists {
		t.Errorf("nonce1 should have been removed from set")
	}
	if _, exists := nb.set[nonce2]; !exists {
		t.Errorf("nonce2 should exist in set")
	}
	if _, exists := nb.set[nonce3]; !exists {
		t.Errorf("nonce3 should exist in set")
	}
}

func TestNextReadUpdate(t *testing.T) {
	nb := NewNonceBuf(2)
	nonce1 := dummyNonceBytes(1, 1)
	nonce2 := dummyNonceBytes(2, 2)

	if err := nb.Insert(nonce1); err != nil {
		t.Fatalf("Insert nonce1 error: %v", err)
	}
	if nb.nextRead != 1 {
		t.Errorf("expected nextRead to be updated to 1, got %d", nb.nextRead)
	}

	if err := nb.Insert(nonce2); err != nil {
		t.Fatalf("Insert nonce2 error: %v", err)
	}
	if nb.nextRead == 2 {
		t.Errorf("expected nextRead to remain 1, got %d", nb.nextRead)
	}
}
