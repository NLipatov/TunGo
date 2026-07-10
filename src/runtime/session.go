package runtime

import "context"

type Session interface {
	WaitForReady(context.Context) error
	Stop()
	Wait() error
}
