package runtime

type Session interface {
	Ready() <-chan struct{}
	Done() <-chan struct{}
	Err() error
	Stop()
	Wait() error
}
