package lifecycle

import (
	"context"
	"errors"
	"sync"
)

var ErrStoppedBeforeReady = errors.New("runtime stopped before becoming ready")

type Session struct {
	readyCh chan struct{}
	doneCh  chan struct{}
	stop    func()

	readyOnce sync.Once
	stopOnce  sync.Once
	err       error
}

func New(stop func()) *Session {
	return &Session{
		readyCh: make(chan struct{}),
		doneCh:  make(chan struct{}),
		stop:    stop,
	}
}

func (s *Session) MarkReady() {
	s.readyOnce.Do(func() {
		close(s.readyCh)
	})
}

func (s *Session) WaitForReady(ctx context.Context) error {
	select {
	case <-s.readyCh:
		return nil
	case <-s.doneCh:
		select {
		case <-s.readyCh:
			return nil
		default:
		}
		if s.err != nil {
			return s.err
		}
		return ErrStoppedBeforeReady
	case <-ctx.Done():
		select {
		case <-s.readyCh:
			return nil
		default:
		}
		return ctx.Err()
	}
}

func (s *Session) Stop() {
	s.stopOnce.Do(s.stop)
}

func (s *Session) Wait() error {
	<-s.doneCh
	return s.err
}

func (s *Session) Finish(err error) {
	s.err = err
	close(s.doneCh)
}
