package tui

import (
	"context"
	"tungo/infrastructure/settings"
	"tungo/runtime"
)

type RuntimeUIOptions struct {
	ReadyCh   <-chan struct{}
	Endpoints []runtime.EndpointInfo
	Protocol  settings.Protocol
}

type runtimeBackend interface {
	enableRuntimeLogCapture(capacity int)
	disableRuntimeLogCapture()
	runRuntimeDashboard(ctx context.Context, mode runtime.Mode, options RuntimeUIOptions) (bool, error)
	setSessionHolder(sh *sessionHolder)
}

type RuntimeUI struct {
	backend runtimeBackend
}

func NewRuntimeUI() *RuntimeUI {
	return newRuntimeUI(newBubbleTeaRuntimeBackend())
}

func newRuntimeUI(backend runtimeBackend) *RuntimeUI {
	if backend == nil {
		backend = newBubbleTeaRuntimeBackend()
	}
	return &RuntimeUI{backend: backend}
}

func (r *RuntimeUI) EnableRuntimeLogCapture(capacity int) {
	r.backend.enableRuntimeLogCapture(capacity)
}

func (r *RuntimeUI) DisableRuntimeLogCapture() {
	r.backend.disableRuntimeLogCapture()
}

func (r *RuntimeUI) RunRuntimeDashboard(ctx context.Context, mode runtime.Mode, options RuntimeUIOptions) (bool, error) {
	return r.backend.runRuntimeDashboard(ctx, mode, options)
}

func (r *RuntimeUI) setSessionHolder(sh *sessionHolder) {
	r.backend.setSessionHolder(sh)
}
