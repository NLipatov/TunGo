package queue

import (
	"io"
	"sync"
)

// RegistrationQueue is a bounded ring buffer of UDPRegistrationPacket.
// It is used only for a single client during handshake.
type RegistrationQueue struct {
	mu     sync.Mutex
	cond   *sync.Cond
	queue  []RegistrationPacket
	head   int
	tail   int
	count  int
	closed bool
}

func NewRegistrationQueue(queueCapacity int) *RegistrationQueue {
	queue := &RegistrationQueue{
		queue: make([]RegistrationPacket, queueCapacity),
	}
	queue.cond = sync.NewCond(&queue.mu)
	return queue
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
	if q.count == len(q.queue) {
		// Registration queue overflow for this client.
		// We drop the packet to avoid blocking global UDP processing.
		return
	}
	slot := &q.queue[q.tail]
	if len(pkt) > len(slot.buffer) {
		// Too large packet for our buffer, drop it.
		return
	}
	slot.n = copy(slot.buffer[:], pkt)
	q.tail = (q.tail + 1) % len(q.queue)
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
	slot := &q.queue[q.head]
	if len(dst) < slot.n {
		return 0, io.ErrShortBuffer
	}
	n := copy(dst, slot.buffer[:slot.n])
	q.head = (q.head + 1) % len(q.queue)
	q.count--
	return n, nil
}

func (q *RegistrationQueue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return
	}
	q.closed = true
	q.cond.Broadcast()
}
