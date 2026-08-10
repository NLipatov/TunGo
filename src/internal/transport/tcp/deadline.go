package tcp

import (
	"io"
	"time"
)

// readDeadlineConn wraps a Transport and refreshes a read deadline before
// each Read call. If the underlying transport does not support SetReadDeadline,
// the wrapper is a no-op pass-through (the original transport is returned).
type readDeadlineConn struct {
	io.ReadWriteCloser
	ds      interface{ SetReadDeadline(time.Time) error }
	timeout time.Duration
}

// WithReadDeadline returns a Transport that sets a read deadline of
// the given timeout before every Read. If t does not support SetReadDeadline,
// t is returned unchanged.
func WithReadDeadline(t io.ReadWriteCloser, timeout time.Duration) io.ReadWriteCloser {
	ds, ok := t.(interface{ SetReadDeadline(time.Time) error })
	if !ok {
		return t
	}
	return &readDeadlineConn{ReadWriteCloser: t, timeout: timeout, ds: ds}
}

func (d *readDeadlineConn) Read(p []byte) (int, error) {
	_ = d.ds.SetReadDeadline(time.Now().Add(d.timeout))
	return d.ReadWriteCloser.Read(p)
}
