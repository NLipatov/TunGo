package network

// ErrTimeout wraps a cause and satisfies net.Error with Timeout()==true.
type ErrTimeout struct{ cause error }

func NewErrTimeout(cause error) *ErrTimeout {
	return &ErrTimeout{cause: cause}
}

func (e ErrTimeout) Error() string   { return e.cause.Error() }
func (e ErrTimeout) Unwrap() error   { return e.cause }
func (e ErrTimeout) Timeout() bool   { return true }
func (e ErrTimeout) Temporary() bool { return false }
