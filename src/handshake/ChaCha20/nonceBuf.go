package ChaCha20

import (
	"fmt"
	"sync"
)

type NonceBuf struct {
	data       []*Nonce
	size       int
	lastInsert int
	nextRead   int
	set        map[uint64]struct{}
	setMu      sync.Mutex
}

func NewNonceBuf(size int) *NonceBuf {
	return &NonceBuf{
		data:       make([]*Nonce, size),
		size:       size,
		lastInsert: -1,
		nextRead:   0,
		set:        make(map[uint64]struct{}),
	}
}

func (r *NonceBuf) Insert(input Nonce) error {
	hash := input.Hash()
	if r.contains(hash) {
		return fmt.Errorf("nonce already exists in buffer: %v", hash)
	}

	r.lastInsert = (r.lastInsert + 1) % r.size

	//if set contains old nonce, remove it from set
	if oldNonce := r.data[r.lastInsert]; oldNonce != nil {
		r.removeFromSet(oldNonce.Hash())
	}

	r.data[r.lastInsert] = &input
	r.addToSet(hash)

	if r.nextRead == r.lastInsert {
		r.nextRead = (r.nextRead + 1) % r.size
	}

	return nil
}

func (r *NonceBuf) contains(hash uint64) bool {
	r.setMu.Lock()
	defer r.setMu.Unlock()
	_, exist := r.set[hash]

	return exist
}

func (r *NonceBuf) addToSet(hash uint64) {
	r.setMu.Lock()
	defer r.setMu.Unlock()
	r.set[hash] = struct{}{}
}

func (r *NonceBuf) removeFromSet(hash uint64) {
	r.setMu.Lock()
	defer r.setMu.Unlock()
	delete(r.set, hash)
}
