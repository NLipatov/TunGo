package queue

type RegQueue interface {
	Enqueue(pkt []byte)
	ReadInto(dst []byte) (int, error)
	Close()
}
