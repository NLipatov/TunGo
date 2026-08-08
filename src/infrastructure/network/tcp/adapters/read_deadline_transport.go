package adapters

import (
	"io"
	"time"
)

// ReadDeadlineTransport wraps a Transport and refreshes a read deadline before
// each Read call. If the underlying transport does not support SetReadDeadline,
// the wrapper is a no-op pass-through (the original transport is returned).
type ReadDeadlineTransport struct {
	io.ReadWriteCloser
	ds      interface{ SetReadDeadline(time.Time) error }
	timeout time.Duration
}

// NewReadDeadlineTransport returns a Transport that sets a read deadline of
// the given timeout before every Read. If t does not support SetReadDeadline,
// t is returned unchanged.
func NewReadDeadlineTransport(t io.ReadWriteCloser, timeout time.Duration) io.ReadWriteCloser {
	ds, ok := t.(interface{ SetReadDeadline(time.Time) error })
	if !ok {
		return t
	}
	return &ReadDeadlineTransport{ReadWriteCloser: t, timeout: timeout, ds: ds}
}

func (d *ReadDeadlineTransport) Read(p []byte) (int, error) {
	_ = d.ds.SetReadDeadline(time.Now().Add(d.timeout))
	return d.ReadWriteCloser.Read(p)
}
