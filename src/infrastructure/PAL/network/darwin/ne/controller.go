//go:build darwin || ios

package ne

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"tungo/application/configuration"
	applicationRuntime "tungo/application/runtime"
	neManager "tungo/infrastructure/PAL/network/darwin/ne/manager"
)

type state uint8

const (
	stateStarting state = iota + 1
	stateRunning
	stateStopped
	stateFailed
)

type Controller struct {
	mu      sync.Mutex
	session *session
}

type session struct {
	mu      sync.Mutex
	runtime applicationRuntime.Runtime
	cancel  context.CancelFunc
	done    chan struct{}
	state   state
	err     error
	release func()
}

func NewController() *Controller {
	return &Controller{}
}

func (c *Controller) NetworkSettings() ([]byte, error) {
	conf, err := configuration.NewDefaultClientControl().ClientRuntimeConfiguration()
	if err != nil {
		return nil, err
	}
	networkSettings, err := NewNetworkSettings(conf)
	if err != nil {
		return nil, err
	}
	return json.Marshal(networkSettings)
}

func (c *Controller) Start(tunnelFD int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session != nil {
		return fmt.Errorf("tunnel is already started")
	}

	release, err := neManager.RegisterFileDescriptor(tunnelFD)
	if err != nil {
		return err
	}
	s := &session{state: stateStopped, release: release}
	if err := s.start(); err != nil {
		release()
		return err
	}
	c.session = s
	return nil
}

func (c *Controller) Stop() error {
	c.mu.Lock()
	s := c.session
	c.mu.Unlock()
	if s == nil {
		return nil
	}
	if err := s.shutdown(); err != nil {
		return err
	}
	c.mu.Lock()
	if c.session == s {
		c.session = nil
	}
	c.mu.Unlock()
	return nil
}

func (c *Controller) WaitReady(ctx context.Context) error {
	c.mu.Lock()
	s := c.session
	c.mu.Unlock()
	if s == nil {
		return fmt.Errorf("tunnel is not started")
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		currentState, sessionErr := s.status()
		switch currentState {
		case stateRunning:
			return nil
		case stateFailed:
			if sessionErr == nil {
				return fmt.Errorf("tunnel startup failed")
			}
			return fmt.Errorf("tunnel startup failed: %w", sessionErr)
		case stateStopped:
			return fmt.Errorf("tunnel stopped before becoming ready")
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for tunnel readiness: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func (s *session) start() error {
	runtimeInstance, err := applicationRuntime.New(applicationRuntime.ModeClient)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.runtime = runtimeInstance
	s.cancel = cancel
	s.done = make(chan struct{})
	s.state = stateStarting
	s.err = nil
	done := s.done
	s.mu.Unlock()

	go func() {
		runErr := runtimeInstance.Run(ctx)
		s.mu.Lock()
		s.cancel = nil
		if ctx.Err() != nil {
			s.state = stateStopped
			s.err = nil
		} else if runErr != nil {
			s.state = stateFailed
			s.err = runErr
		} else {
			s.state = stateStopped
			s.err = nil
		}
		s.mu.Unlock()
		close(done)
	}()
	return nil
}

func (s *session) shutdown() error {
	s.mu.Lock()
	cancel := s.cancel
	done := s.done
	release := s.release
	s.release = nil
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	if release != nil {
		release()
	}
	return nil
}

func (s *session) status() (state, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == stateStarting && s.runtime != nil && s.runtime.Ready() {
		s.state = stateRunning
	}
	return s.state, s.err
}
