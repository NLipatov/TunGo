package chacha20

import (
	"encoding/binary"
	"sync"
)

type NonceBuf struct {
	data       []*Nonce
	size       int
	lastInsert int
	nextRead   int
	set        map[[12]byte]struct{}
	setMu      sync.Mutex
	keyBuf     [12]byte
}

func NewNonceBuf(size int) *NonceBuf {
	return &NonceBuf{
		data:       make([]*Nonce, size),
		size:       size,
		lastInsert: -1,
		nextRead:   0,
		set:        make(map[[12]byte]struct{}),
	}
}

func (r *NonceBuf) InsertNonceBytes(data []byte) error {
	if len(data) != 12 {
		return InvalidNonce
	}

	low := binary.BigEndian.Uint64(data[:8])
	high := binary.BigEndian.Uint32(data[8:])

	hash := [12]byte(data)
	if r.contains(hash) {
		return ErrNonUniqueNonce
	}

	r.lastInsert = (r.lastInsert + 1) % r.size

	//if set contains old nonce, remove it from set
	if oldNonce := r.data[r.lastInsert]; oldNonce != nil {
		r.removeFromSet(oldNonce.Hash(r.keyBuf))
	}

	r.data[r.lastInsert] = &Nonce{
		low:  low,
		high: high,
		mu:   sync.Mutex{},
	}
	r.addToSet(hash)

	if r.nextRead == r.lastInsert {
		r.nextRead = (r.nextRead + 1) % r.size
	}

	return nil
}

func (r *NonceBuf) Insert(input *Nonce) error {
	hash := input.Hash(r.keyBuf)
	if r.contains(hash) {
		return ErrNonUniqueNonce
	}

	r.lastInsert = (r.lastInsert + 1) % r.size

	//if set contains old nonce, remove it from set
	if oldNonce := r.data[r.lastInsert]; oldNonce != nil {
		r.removeFromSet(oldNonce.Hash(r.keyBuf))
	}

	r.data[r.lastInsert] = input
	r.addToSet(hash)

	if r.nextRead == r.lastInsert {
		r.nextRead = (r.nextRead + 1) % r.size
	}

	return nil
}

func (r *NonceBuf) contains(key [12]byte) bool {
	r.setMu.Lock()
	defer r.setMu.Unlock()
	_, exist := r.set[key]

	return exist
}

func (r *NonceBuf) addToSet(key [12]byte) {
	r.setMu.Lock()
	defer r.setMu.Unlock()
	r.set[key] = struct{}{}
}

func (r *NonceBuf) removeFromSet(key [12]byte) {
	r.setMu.Lock()
	defer r.setMu.Unlock()
	delete(r.set, key)
}
