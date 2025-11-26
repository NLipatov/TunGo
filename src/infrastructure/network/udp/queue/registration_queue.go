package queue

import (
	"io"
	"sync"
	"tungo/infrastructure/settings"
)

// RegistrationPacket holds a single UDP datagram for a registering client.
// The buffer is preallocated and reused, so no per-packet allocations happen
// once the queue is created.
type RegistrationPacket struct {
	n    int
	data [settings.DefaultEthernetMTU + settings.UDPChacha20Overhead]byte
}

// RegistrationQueue is a bounded ring buffer of RegistrationPacket.
// It is used only for a single client during handshake.
type RegistrationQueue struct {
	mu     sync.Mutex
	cond   *sync.Cond
	buf    []RegistrationPacket
	head   int
	tail   int
	count  int
	closed bool
}

func NewRegistrationQueue(capacity int) *RegistrationQueue {
	q := &RegistrationQueue{
		buf: make([]RegistrationPacket, capacity),
	}
	q.cond = sync.NewCond(&q.mu)
	return q
}

// Enqueue copies the packet into the per-client buffer if there is space.
// If the queue is full, the packet for this client is dropped, but other
// clients remain unaffected.
func (q *RegistrationQueue) Enqueue(pkt []byte) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return
	}

	if q.count == len(q.buf) {
		// Registration queue overflow for this client.
		// We drop the packet to avoid blocking global UDP processing.
		return
	}

	slot := &q.buf[q.tail]
	if len(pkt) > len(slot.data) {
		// Too large packet for our buffer, drop it.
		return
	}

	slot.n = copy(slot.data[:], pkt)

	q.tail = (q.tail + 1) % len(q.buf)
	q.count++
	q.cond.Signal()
}

// ReadInto blocks until there is a packet or the queue is closed.
// It copies the next datagram into dst and returns its length.
func (q *RegistrationQueue) ReadInto(dst []byte) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for q.count == 0 && !q.closed {
		q.cond.Wait()
	}

	if q.count == 0 && q.closed {
		return 0, io.EOF
	}

	slot := &q.buf[q.head]
	if len(dst) < slot.n {
		return 0, io.ErrShortBuffer
	}

	n := copy(dst, slot.data[:slot.n])

	q.head = (q.head + 1) % len(q.buf)
	q.count--

	return n, nil
}

func (q *RegistrationQueue) Close() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	q.cond.Broadcast()
}
