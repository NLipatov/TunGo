package chacha20

import (
	"errors"
	"sync"
	"testing"
)

// TestNonceBuf_Insert_Unique unique nonce should be inserted
func TestNonceBuf_Insert_Unique(t *testing.T) {
	bufSize := 5
	nonceBuf := NewNonceBuf(bufSize)

	for i := 0; i < bufSize; i++ {
		nonce := Nonce{Low: uint64(i), High: 0}
		err := nonceBuf.Insert(nonce)
		if err != nil {
			t.Errorf("Failed to insert unique nonce: %v", err)
		}
	}
}

// TestNonceBuf_Insert_Duplicate non-unique nonce should not be inserted
func TestNonceBuf_Insert_Duplicate(t *testing.T) {
	nonceBuf := NewNonceBuf(5)
	nonce := Nonce{Low: 12345, High: 0}

	err := nonceBuf.Insert(nonce)
	if err != nil {
		t.Errorf("Failed to insert nonce: %v", err)
	}

	//should be a ErrNonUniqueNonce here, as nonce is attempted to be inserted for second time
	err = nonceBuf.Insert(nonce)
	if !errors.Is(err, ErrNonUniqueNonce) {
		t.Errorf("Expected ErrNonUniqueNonce, got: %v", err)
	}
}

// TestNonceBuf_Overwrite when NonceBuf has no empty space, write index should be 0 again
func TestNonceBuf_Overwrite(t *testing.T) {
	bufSize := 3
	nonceBuf := NewNonceBuf(bufSize)

	values := []Nonce{
		{Low: 1, High: 0},
		{Low: 2, High: 0},
		{Low: 3, High: 0},
	}

	for _, nonce := range values {
		err := nonceBuf.Insert(nonce)
		if err != nil {
			t.Errorf("Failed to insert nonce: %v", err)
		}
	}

	newNonce := Nonce{Low: 4, High: 0}
	err := nonceBuf.Insert(newNonce)
	if err != nil {
		t.Errorf("Failed to insert new nonce: %v", err)
	}

	err = nonceBuf.Insert(values[0])
	if err != nil {
		t.Errorf("Expected to insert old nonce again, but got error: %v", err)
	}
}

// TestNonceBuf_ConcurrentInsert  checks if concurrent insert is working correctly
func TestNonceBuf_ConcurrentInsert(t *testing.T) {
	nonceBuf := NewNonceBuf(100)
	var wg sync.WaitGroup
	numGoroutines := 10
	nonceCountPerGoroutine := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for j := 0; j < nonceCountPerGoroutine; j++ {
				nonce := Nonce{Low: uint64(gid*nonceCountPerGoroutine + j), High: 0}
				err := nonceBuf.Insert(nonce)
				if err != nil {
					t.Errorf("Goroutine %d: Failed to insert nonce %v: %v", gid, nonce, err)
				}
			}
		}(i)
	}

	wg.Wait()
}

// TestNonceBuf_Insert_AfterOverwrite
func TestNonceBuf_Insert_AfterOverwrite(t *testing.T) {
	bufSize := 3
	nonceBuf := NewNonceBuf(bufSize)

	for i := 0; i < bufSize; i++ {
		nonce := Nonce{Low: uint64(i), High: 0}
		err := nonceBuf.Insert(nonce)
		if err != nil {
			t.Errorf("Failed to insert nonce: %v", err)
		}
	}

	for i := bufSize; i < bufSize*2; i++ {
		nonce := Nonce{Low: uint64(i), High: 0}
		err := nonceBuf.Insert(nonce)
		if err != nil {
			t.Errorf("Failed to insert nonce: %v", err)
		}
	}

	for i := 0; i < bufSize; i++ {
		nonce := Nonce{Low: uint64(i), High: 0}
		err := nonceBuf.Insert(nonce)
		if err != nil {
			t.Errorf("Expected to insert old nonce again, but got error: %v", err)
		}
	}
}
