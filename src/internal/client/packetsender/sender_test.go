package packetsender_test

import (
	"errors"
	"fmt"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"tungo/internal/client/packetsender"
)

type encryptFunc func([]byte) ([]byte, error)

func (f encryptFunc) Encrypt(plaintext []byte) ([]byte, error) {
	return f(plaintext)
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(ciphertext []byte) (int, error) {
	return f(ciphertext)
}

func TestSendSerializesEncryptionAndWrite(t *testing.T) {
	const count = 32
	var inFlight atomic.Bool
	written := make(chan string, count)
	sender := packetsender.New(
		writerFunc(func(ciphertext []byte) (int, error) {
			// Let competing sends run while this packet is still being written.
			runtime.Gosched()
			written <- string(ciphertext)
			inFlight.Store(false)
			return len(ciphertext), nil
		}),
		encryptFunc(func(plaintext []byte) ([]byte, error) {
			if !inFlight.CompareAndSwap(false, true) {
				return nil, errors.New("encryption started before the previous write completed")
			}
			runtime.Gosched()
			return []byte("encrypted:" + string(plaintext)), nil
		}),
	)

	start := make(chan struct{})
	var sends sync.WaitGroup
	for id := range count {
		sends.Go(func() {
			<-start
			if err := sender.Send(fmt.Appendf(nil, "packet-%d", id)); err != nil {
				t.Errorf("Send(%d): %v", id, err)
			}
		})
	}
	close(start)
	sends.Wait()
	close(written)

	seen := make(map[string]int, count)
	for ciphertext := range written {
		seen[ciphertext]++
	}
	for id := range count {
		packet := fmt.Sprintf("encrypted:packet-%d", id)
		if got := seen[packet]; got != 1 {
			t.Errorf("writes of %q = %d, want 1", packet, got)
		}
	}
}

func TestSendReturnsErrorAndAllowsNextSend(t *testing.T) {
	for _, failAt := range []string{"encrypt", "write"} {
		t.Run(failAt, func(t *testing.T) {
			failure := errors.New("send failed")
			var written []string
			sender := packetsender.New(
				writerFunc(func(ciphertext []byte) (int, error) {
					written = append(written, string(ciphertext))
					if failAt == "write" && string(ciphertext) == "encrypted:first" {
						return 0, failure
					}
					return len(ciphertext), nil
				}),
				encryptFunc(func(plaintext []byte) ([]byte, error) {
					if failAt == "encrypt" && string(plaintext) == "first" {
						return nil, failure
					}
					return []byte("encrypted:" + string(plaintext)), nil
				}),
			)

			results := make(chan error, 2)
			go func() {
				results <- sender.Send([]byte("first"))
				results <- sender.Send([]byte("second"))
			}()
			for attempt, want := range []error{failure, nil} {
				select {
				case err := <-results:
					if !errors.Is(err, want) {
						t.Fatalf("Send attempt %d: error = %v, want %v", attempt+1, err, want)
					}
				case <-time.After(2 * time.Second):
					t.Fatalf("Send attempt %d did not complete", attempt+1)
				}
			}

			want := []string{"encrypted:second"}
			if failAt == "write" {
				want = []string{"encrypted:first", "encrypted:second"}
			}
			if !slices.Equal(written, want) {
				t.Fatalf("Write calls = %q, want %q", written, want)
			}
		})
	}
}
