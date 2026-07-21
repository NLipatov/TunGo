//go:build darwin || ios

package apple

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"tungo/application/configuration"
	applicationRuntime "tungo/application/runtime"
	tunnelClient "tungo/infrastructure/PAL/tunnel/client"
)

type State int32

const (
	StateStarting State = iota + 1
	StateRunning
	StateStopped
	StateFailed
)

type Status struct {
	State State
	Error string
}

type Controller struct {
	mu       sync.Mutex
	next     uint64
	sessions map[uint64]*session
}

type session struct {
	lifecycle  sync.Mutex
	terminal   bool
	mu         sync.Mutex
	generation uint64
	runtime    applicationRuntime.Runtime
	cancel     context.CancelFunc
	done       chan struct{}
	state      State
	err        error
	release    func()
}

func NewController() *Controller {
	return &Controller{sessions: make(map[uint64]*session)}
}

func (c *Controller) TunnelPlan() ([]byte, error) {
	conf, err := configuration.NewDefaultClientControl().ClientRuntimeConfiguration()
	if err != nil {
		return nil, err
	}
	plan, err := NewTunnelPlan(conf)
	if err != nil {
		return nil, err
	}
	return json.Marshal(plan)
}

func (c *Controller) Start(tunnelFD int) (uint64, error) {
	release, err := tunnelClient.RegisterNetworkExtensionFileDescriptor(tunnelFD)
	if err != nil {
		return 0, err
	}
	s := &session{state: StateStopped, release: release}
	if err := s.start(); err != nil {
		release()
		return 0, err
	}

	c.mu.Lock()
	c.next++
	handle := c.next
	c.sessions[handle] = s
	c.mu.Unlock()
	return handle, nil
}

func (c *Controller) Stop(handle uint64) error {
	s, err := c.lookup(handle)
	if err != nil {
		return err
	}
	if err := s.shutdown(); err != nil {
		return err
	}
	c.mu.Lock()
	if c.sessions[handle] == s {
		delete(c.sessions, handle)
	}
	c.mu.Unlock()
	return nil
}

func (c *Controller) Pause(handle uint64) error {
	s, err := c.lookup(handle)
	if err != nil {
		return err
	}
	return s.stop()
}

func (c *Controller) Restart(handle uint64) error {
	s, err := c.lookup(handle)
	if err != nil {
		return err
	}
	return s.restart()
}

func (c *Controller) Status(handle uint64) (Status, error) {
	s, err := c.lookup(handle)
	if err != nil {
		return Status{}, err
	}
	return s.status(), nil
}

func (c *Controller) WaitReady(ctx context.Context, handle uint64) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		status, err := c.Status(handle)
		if err != nil {
			return err
		}
		switch status.State {
		case StateRunning:
			return nil
		case StateFailed:
			if status.Error == "" {
				return fmt.Errorf("tunnel startup failed")
			}
			return fmt.Errorf("tunnel startup failed: %s", status.Error)
		case StateStopped:
			return fmt.Errorf("tunnel stopped before becoming ready")
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for tunnel readiness: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func (c *Controller) lookup(handle uint64) (*session, error) {
	c.mu.Lock()
	s := c.sessions[handle]
	c.mu.Unlock()
	if s == nil {
		return nil, fmt.Errorf("unknown tunnel handle %d", handle)
	}
	return s, nil
}

func (s *session) start() error {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	return s.startLocked()
}

func (s *session) startLocked() error {
	if s.terminal {
		return fmt.Errorf("tunnel has been stopped")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == StateStarting || s.state == StateRunning {
		return fmt.Errorf("tunnel is already running")
	}

	runtimeInstance, err := applicationRuntime.New(applicationRuntime.ModeClient)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.generation++
	generation := s.generation
	s.runtime = runtimeInstance
	s.cancel = cancel
	s.done = make(chan struct{})
	s.state = StateStarting
	s.err = nil

	go func(done chan struct{}) {
		runErr := runtimeInstance.Run(ctx)
		s.mu.Lock()
		if s.generation == generation {
			s.cancel = nil
			if ctx.Err() != nil {
				s.state = StateStopped
				s.err = nil
			} else if runErr != nil {
				s.state = StateFailed
				s.err = runErr
			} else {
				s.state = StateStopped
				s.err = nil
			}
		}
		s.mu.Unlock()
		close(done)
	}(s.done)
	return nil
}

func (s *session) stop() error {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	return s.stopLocked()
}

func (s *session) shutdown() error {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	s.terminal = true
	err := s.stopLocked()
	if s.release != nil {
		s.release()
		s.release = nil
	}
	return err
}

func (s *session) restart() error {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	if s.terminal {
		return fmt.Errorf("tunnel has been stopped")
	}
	if err := s.stopLocked(); err != nil {
		return err
	}
	return s.startLocked()
}

func (s *session) stopLocked() error {
	s.mu.Lock()
	if s.state == StateStopped {
		s.mu.Unlock()
		return nil
	}
	cancel := s.cancel
	done := s.done
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	return nil
}

func (s *session) status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.state
	if state == StateStarting && s.runtime != nil && s.runtime.Ready() {
		state = StateRunning
		s.state = StateRunning
	}
	status := Status{State: state}
	if s.err != nil {
		status.Error = s.err.Error()
	}
	return status
}
