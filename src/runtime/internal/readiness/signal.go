package readiness

import (
	"context"
	"sync"
)

type Signal struct {
	ch   chan struct{}
	once sync.Once
}

func NewSignal() *Signal {
	return &Signal{ch: make(chan struct{})}
}

func (s *Signal) Mark() {
	s.once.Do(func() {
		close(s.ch)
	})
}

func (s *Signal) Wait(ctx context.Context) error {
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
