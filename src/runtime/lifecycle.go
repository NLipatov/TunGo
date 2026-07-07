package runtime

import (
	"context"
	"errors"
	"sync"
)

type Session interface {
	Ready() <-chan struct{}
	Done() <-chan struct{}
	Err() error
	Stop()
	Wait() error
}

type runningSession struct {
	ctx     context.Context
	readyCh <-chan struct{}
	doneCh  chan struct{}
	stop    context.CancelFunc

	mu  sync.Mutex
	err error
}

func newRunningSession(
	ctx context.Context,
	readyCh <-chan struct{},
	stop context.CancelFunc,
) *runningSession {
	if ctx == nil {
		ctx = context.Background()
	}
	return &runningSession{
		ctx:     ctx,
		readyCh: readyCh,
		doneCh:  make(chan struct{}),
		stop:    stop,
	}
}

func (s *runningSession) Ready() <-chan struct{} {
	return s.readyCh
}

func (s *runningSession) Done() <-chan struct{} {
	return s.doneCh
}

func (s *runningSession) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *runningSession) Stop() {
	s.stop()
}

func (s *runningSession) Wait() error {
	<-s.doneCh
	return terminalErrOrNil(s.ctx, s.Err())
}

func terminalErrOrNil(ctx context.Context, err error) error {
	if err != nil && ctx.Err() == nil &&
		!errors.Is(err, context.Canceled) &&
		!errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return nil
}

func (s *runningSession) finish(err error) {
	s.mu.Lock()
	s.err = err
	s.mu.Unlock()
	close(s.doneCh)
}
