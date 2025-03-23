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

	if _, exists := nb.set[nonce1]; !exists {
		t.Errorf("expected nonce1 to be in set")
	}
	if _, exists := nb.set[nonce2]; !exists {
		t.Errorf("expected nonce2 to be in set")
	}
	if _, exists := nb.set[nonce3]; !exists {
		t.Errorf("expected nonce3 to be in set")
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
	// Expect nextRead to update to 1.
	if nb.nextRead != 1 {
		t.Errorf("expected nextRead to be updated to 1, got %d", nb.nextRead)
	}

	if err := nb.Insert(nonce2); err != nil {
		t.Fatalf("Insert nonce2 error: %v", err)
	}
	// After wrap-around, nextRead should update accordingly.
	if nb.nextRead != 0 {
		t.Errorf("expected nextRead to be updated to 0, got %d", nb.nextRead)
	}
}

func TestNonceRulesChacha20(t *testing.T) {
	nb := NewNonceBuf(3)
	a := dummyNonceBytes(1, 1)
	b := dummyNonceBytes(2, 2)
	c := dummyNonceBytes(3, 3)

	// Insert three unique nonces.
	if err := nb.Insert(a); err != nil {
		t.Fatalf("insert a failed: %v", err)
	}
	if err := nb.Insert(b); err != nil {
		t.Fatalf("insert b failed: %v", err)
	}
	if err := nb.Insert(c); err != nil {
		t.Fatalf("insert c failed: %v", err)
	}

	// Duplicate insertion of 'a' should fail.
	if err := nb.Insert(a); err == nil || !errors.Is(err, ErrNonUniqueNonce) {
		t.Fatalf("expected error on duplicate insert of a, got: %v", err)
	}

	// Inserting a new nonce forces wrap-around, evicting the oldest value (a).
	d := dummyNonceBytes(4, 4)
	if err := nb.Insert(d); err != nil {
		t.Fatalf("insert d failed: %v", err)
	}

	// 'a' was evicted so it can be reinserted.
	if err := nb.Insert(a); err != nil {
		t.Fatalf("insert a after eviction failed: %v", err)
	}
}
