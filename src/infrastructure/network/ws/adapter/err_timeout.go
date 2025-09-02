package adapter

// errTimeout wraps a cause and satisfies net.Error with Timeout()==true.
type errTimeout struct{ cause error }

func (e errTimeout) Error() string   { return e.cause.Error() }
func (e errTimeout) Unwrap() error   { return e.cause }
func (e errTimeout) Timeout() bool   { return true }
func (e errTimeout) Temporary() bool { return false }
