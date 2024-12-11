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
		nonce := &Nonce{low: uint64(i), high: 0}
		err := nonceBuf.Insert(nonce)
		if err != nil {
			t.Errorf("Failed to insert unique nonce: %v", err)
		}
	}
}

// TestNonceBuf_Insert_Duplicate non-unique nonce should not be inserted
func TestNonceBuf_Insert_Duplicate(t *testing.T) {
	nonceBuf := NewNonceBuf(5)
	nonce := &Nonce{low: 12345, high: 0}

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

	values := []*Nonce{
		{low: 1, high: 0},
		{low: 2, high: 0},
		{low: 3, high: 0},
	}

	for _, nonce := range values {
		err := nonceBuf.Insert(nonce)
		if err != nil {
			t.Errorf("Failed to insert nonce: %v", err)
		}
	}

	newNonce := &Nonce{low: 4, high: 0}
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
				nonce := &Nonce{low: uint64(gid*nonceCountPerGoroutine + j), high: 0}
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
		nonce := &Nonce{low: uint64(i), high: 0}
		err := nonceBuf.Insert(nonce)
		if err != nil {
			t.Errorf("Failed to insert nonce: %v", err)
		}
	}

	for i := bufSize; i < bufSize*2; i++ {
		nonce := &Nonce{low: uint64(i), high: 0}
		err := nonceBuf.Insert(nonce)
		if err != nil {
			t.Errorf("Failed to insert nonce: %v", err)
		}
	}

	for i := 0; i < bufSize; i++ {
		nonce := &Nonce{low: uint64(i), high: 0}
		err := nonceBuf.Insert(nonce)
		if err != nil {
			t.Errorf("Expected to insert old nonce again, but got error: %v", err)
		}
	}
}
