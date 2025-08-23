package internal

import (
	"context"
	"time"
)

// CtxFactory creates deadline-bound contexts. Default uses context.WithDeadline.
type CtxFactory interface {
	WithDeadline(parent context.Context, d time.Time) (context.Context, context.CancelFunc)
}

type DefaultCtxFactory struct{}

func (DefaultCtxFactory) WithDeadline(parent context.Context, d time.Time) (context.Context, context.CancelFunc) {
	return context.WithDeadline(parent, d)
}
