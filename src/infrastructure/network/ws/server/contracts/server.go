package contracts

type Server interface {
	Serve() error
	Shutdown() error
	Done() <-chan struct{}
	Err() error
}
