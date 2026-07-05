package runtime

import (
	"context"
	"sync"
	"tungo/infrastructure/settings"
)

type Lifecycle interface {
	Start(ctx context.Context, mode Mode) (Session, error)
}

type Session interface {
	Info() Info
	Ready() <-chan struct{}
	Done() <-chan struct{}
	Err() error
	Stop()
}

type Info struct {
	Mode      Mode
	Endpoints []EndpointInfo
	Protocol  settings.Protocol
}

type RunningSession struct {
	info    Info
	readyCh <-chan struct{}
	doneCh  chan struct{}
	stop    context.CancelFunc

	mu  sync.Mutex
	err error
}

func NewRunningSession(
	info Info,
	readyCh <-chan struct{},
	stop context.CancelFunc,
) *RunningSession {
	return &RunningSession{
		info:    info,
		readyCh: readyCh,
		doneCh:  make(chan struct{}),
		stop:    stop,
	}
}

func (s *RunningSession) Info() Info {
	return s.info
}

func (s *RunningSession) Ready() <-chan struct{} {
	return s.readyCh
}

func (s *RunningSession) Done() <-chan struct{} {
	return s.doneCh
}

func (s *RunningSession) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *RunningSession) Stop() {
	s.stop()
}

func (s *RunningSession) Finish(err error) {
	s.mu.Lock()
	s.err = err
	s.mu.Unlock()
	close(s.doneCh)
}
