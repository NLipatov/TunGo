package chacha20

import (
	"encoding/binary"
	"sync"
	"unsafe"
)

type NonceBuf struct {
	data       []*Nonce
	size       int
	lastInsert int
	nextRead   int
	set        map[[12]byte]struct{}
	keyBuf     [12]byte
}

func NewNonceBuf(size int) *NonceBuf {
	return &NonceBuf{
		data:       make([]*Nonce, size),
		size:       size,
		lastInsert: -1,
		nextRead:   0,
		set:        make(map[[12]byte]struct{}, size),
	}
}

func (r *NonceBuf) Insert(nonceBytes [12]byte) error {
	low := binary.BigEndian.Uint64(nonceBytes[:8])
	high := binary.BigEndian.Uint32(nonceBytes[8:])

	hash := nonceBytes
	_, exist := r.set[hash]
	if exist {
		return ErrNonUniqueNonce
	}

	r.lastInsert = (r.lastInsert + 1) % r.size

	//if set contains old nonce, remove it from set
	if oldNonce := r.data[r.lastInsert]; oldNonce != nil {
		key := *(*[12]byte)(unsafe.Pointer(&oldNonce.Encode(r.keyBuf[:])[0]))
		delete(r.set, key)
	}

	r.data[r.lastInsert] = &Nonce{
		low:  low,
		high: high,
		mu:   sync.Mutex{},
	}
	r.set[hash] = struct{}{}

	if r.nextRead == r.lastInsert {
		r.nextRead = (r.nextRead + 1) % r.size
	}

	return nil
}
