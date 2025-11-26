package udp

import (
	"io"
	"sync"
	"testing"
	"time"
)

// mockRegistrationQueueWaiter is a helper used to synchronize goroutines.
// It is NOT mocking the queue internals, only behaviour around Wait().
type mockRegistrationQueueWaiter struct {
	wg sync.WaitGroup
}

func (m *mockRegistrationQueueWaiter) Block() {
	m.wg.Add(1)
	m.wg.Wait()
}

func (m *mockRegistrationQueueWaiter) Release() {
	m.wg.Done()
}

func TestRegistrationQueue_Table(t *testing.T) {
	tests := []struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "enqueue_and_read_single_packet",
			fn: func(t *testing.T) {
				// Ensures that a basic enqueue→read works in concurrent mode.
				q := NewRegistrationQueue(2)
				data := []byte("hello")
				dst := make([]byte, 16)

				go func() {
					time.Sleep(10 * time.Millisecond)
					q.Enqueue(data)
				}()

				n, err := q.ReadInto(dst)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if n != len(data) {
					t.Fatalf("unexpected length: got %d want %d", n, len(data))
				}
				if string(dst[:n]) != string(data) {
					t.Fatalf("unexpected data: %q", string(dst[:n]))
				}
			},
		},

		{
			name: "read_blocks_until_enqueue",
			fn: func(t *testing.T) {
				// Confirms Wait() blocks until enqueue happens.
				q := NewRegistrationQueue(1)
				dst := make([]byte, 8)

				done := make(chan struct{})

				go func() {
					n, err := q.ReadInto(dst)
					if err != nil {
						t.Errorf("unexpected error: %v", err)
					}
					if n != 3 || string(dst[:n]) != "one" {
						t.Errorf("bad data: %q", string(dst[:n]))
					}
					close(done)
				}()

				time.Sleep(20 * time.Millisecond)
				q.Enqueue([]byte("one"))

				select {
				case <-done:
				case <-time.After(time.Second):
					t.Fatal("read did not unblock")
				}
			},
		},

		{
			name: "close_unblocks_reader_with_eof",
			fn: func(t *testing.T) {
				// Confirms that Close() wakes goroutine with EOF.
				q := NewRegistrationQueue(1)
				dst := make([]byte, 4)

				done := make(chan struct{})

				go func() {
					_, err := q.ReadInto(dst)
					if err != io.EOF {
						t.Errorf("expected EOF, got: %v", err)
					}
					close(done)
				}()

				time.Sleep(20 * time.Millisecond)
				q.Close()

				select {
				case <-done:
				case <-time.After(time.Second):
					t.Fatal("reader not unblocked on close")
				}
			},
		},

		{
			name: "queue_full_drops_packets",
			fn: func(t *testing.T) {
				q := NewRegistrationQueue(1)
				q.Enqueue([]byte("first"))
				q.Enqueue([]byte("second")) // should be dropped

				dst := make([]byte, 16)
				n, err := q.ReadInto(dst)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if string(dst[:n]) != "first" {
					t.Errorf("unexpected data: %q", string(dst[:n]))
				}

				// No second packet expected.
				done := make(chan struct{})
				go func() {
					_, err := q.ReadInto(dst)
					if err != io.EOF {
						t.Errorf("expected EOF, got %v", err)
					}
					close(done)
				}()
				q.Close()
				<-done
			},
		},

		{
			name: "packet_too_large_is_dropped",
			fn: func(t *testing.T) {
				q := NewRegistrationQueue(1)
				big := make([]byte, len(q.queue[0].buffer)+1)

				q.Enqueue(big) // drop
				q.Enqueue([]byte("ok"))

				dst := make([]byte, 16)
				n, _ := q.ReadInto(dst)

				if string(dst[:n]) != "ok" {
					t.Errorf("bad packet: %q", string(dst[:n]))
				}
			},
		},

		{
			name: "read_into_short_buffer_returns_error",
			fn: func(t *testing.T) {
				q := NewRegistrationQueue(1)
				q.Enqueue([]byte("hello"))

				dst := make([]byte, 2) // too small
				_, err := q.ReadInto(dst)
				if err != io.ErrShortBuffer {
					t.Fatalf("expected ErrShortBuffer, got %v", err)
				}
			},
		},

		{
			name: "close_multiple_times_is_safe",
			fn: func(t *testing.T) {
				q := NewRegistrationQueue(1)
				q.Close()
				q.Close()
				q.Close()
			},
		},

		{
			name: "enqueue_after_close_is_ignored",
			fn: func(t *testing.T) {
				q := NewRegistrationQueue(1)
				q.Close()
				q.Enqueue([]byte("x"))

				dst := make([]byte, 4)

				_, err := q.ReadInto(dst)
				if err != io.EOF {
					t.Fatalf("expected EOF, got %v", err)
				}
			},
		},

		{
			name: "multiple_concurrent_readers_and_writers",
			fn: func(t *testing.T) {
				// Stress test concurrency correctness.
				q := NewRegistrationQueue(8)
				dst := make([]byte, 8)

				var wg sync.WaitGroup

				// Writers
				for i := 0; i < 5; i++ {
					wg.Add(1)
					go func(i int) {
						defer wg.Done()
						q.Enqueue([]byte{byte(i + 1)})
					}(i)
				}

				// Readers
				for i := 0; i < 5; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						q.ReadInto(dst)
					}()
				}

				wg.Wait()
				q.Close()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.fn)
	}
}
