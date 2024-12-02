package ChaCha20

import (
	"sync"
)

type NonceBuf struct {
	data       []*Nonce
	size       int
	lastInsert int
	nextRead   int
	set        map[string]struct{}
	setMu      sync.Mutex
}

func NewNonceBuf(size int) *NonceBuf {
	return &NonceBuf{
		data:       make([]*Nonce, size),
		size:       size,
		lastInsert: -1,
		nextRead:   0,
		set:        make(map[string]struct{}),
	}
}

func (r *NonceBuf) Insert(input Nonce) error {
	hash := input.Hash()
	if r.contains(hash) {
		return ErrNonUniqueNonce
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

func (r *NonceBuf) contains(key string) bool {
	r.setMu.Lock()
	defer r.setMu.Unlock()
	_, exist := r.set[key]

	return exist
}

func (r *NonceBuf) addToSet(key string) {
	r.setMu.Lock()
	defer r.setMu.Unlock()
	r.set[key] = struct{}{}
}

func (r *NonceBuf) removeFromSet(key string) {
	r.setMu.Lock()
	defer r.setMu.Unlock()
	delete(r.set, key)
}
