package runtime

import (
	"context"
	"sync"
)

type readySignal struct {
	ch   chan struct{}
	once sync.Once
}

func newReadySignal() *readySignal {
	return &readySignal{ch: make(chan struct{})}
}

func (s *readySignal) mark() {
	s.once.Do(func() {
		close(s.ch)
	})
}

func (s *readySignal) wait(ctx context.Context) error {
	select {
	case <-s.ch:
		return nil
	case <-ctx.Done():
		select {
		case <-s.ch:
			return nil
		default:
		}
		return ctx.Err()
	}
}
