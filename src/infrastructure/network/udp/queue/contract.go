package queue

type RegistrationQueue interface {
	Enqueue(pkt []byte)
	ReadInto(dst []byte) (int, error)
	Close()
}
