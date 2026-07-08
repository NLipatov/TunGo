package lifecycle

import (
	"context"
	"errors"
	"sync"
)

type Session struct {
	ctx     context.Context
	readyCh <-chan struct{}
	doneCh  chan struct{}
	stop    context.CancelFunc

	mu  sync.Mutex
	err error
}

func New(
	ctx context.Context,
	readyCh <-chan struct{},
	stop context.CancelFunc,
) *Session {
	if ctx == nil {
		ctx = context.Background()
	}
	return &Session{
		ctx:     ctx,
		readyCh: readyCh,
		doneCh:  make(chan struct{}),
		stop:    stop,
	}
}

func (s *Session) Ready() <-chan struct{} {
	return s.readyCh
}

func (s *Session) Done() <-chan struct{} {
	return s.doneCh
}

func (s *Session) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *Session) Stop() {
	s.stop()
}

func (s *Session) Wait() error {
	<-s.doneCh
	return terminalErrOrNil(s.ctx, s.Err())
}

func (s *Session) Finish(err error) {
	s.mu.Lock()
	s.err = err
	s.mu.Unlock()
	close(s.doneCh)
}

func ClosedReadyCh() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func terminalErrOrNil(ctx context.Context, err error) error {
	if err != nil && ctx.Err() == nil &&
		!errors.Is(err, context.Canceled) &&
		!errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return nil
}
