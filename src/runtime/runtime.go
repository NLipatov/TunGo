package runtime

import "context"

// Runtime is a single-use runtime instance.
type Runtime interface {
	// Run blocks until the runtime stops. Context cancellation is a clean stop;
	// operational failures are returned as errors.
	Run(context.Context) error
	// WaitForReady blocks until the runtime can serve traffic or ctx ends.
	WaitForReady(context.Context) error
}
