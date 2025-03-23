package chacha20

type NonceBuf struct {
	data       [][12]byte
	size       int
	lastInsert int
	nextRead   int
	set        map[[12]byte]struct{}
	keyBuf     [12]byte
}

func NewNonceBuf(size int) *NonceBuf {
	return &NonceBuf{
		data:       make([][12]byte, size),
		size:       size,
		lastInsert: -1,
		nextRead:   0,
		set:        make(map[[12]byte]struct{}, size),
	}
}

func (r *NonceBuf) Insert(nonceBytes [12]byte) error {
	_, notUnique := r.set[nonceBytes]
	if notUnique {
		return ErrNonUniqueNonce
	}

	r.lastInsert = (r.lastInsert + 1) % r.size

	oldNonce := r.data[r.lastInsert]
	if _, exists := r.set[oldNonce]; exists {
		delete(r.set, oldNonce)
	}

	r.data[r.lastInsert] = nonceBytes
	r.set[nonceBytes] = struct{}{}

	if r.nextRead == r.lastInsert {
		r.nextRead = (r.nextRead + 1) % r.size
	}

	return nil
}
