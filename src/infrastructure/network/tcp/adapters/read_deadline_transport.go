package adapters

import (
	"time"
	"tungo/application/network/connection"
)

// ReadDeadlineTransport wraps a Transport and refreshes a read deadline before
// each Read call. If the underlying transport does not support SetReadDeadline,
// the wrapper is a no-op pass-through (the original transport is returned).
type ReadDeadlineTransport struct {
	connection.Transport
	ds      interface{ SetReadDeadline(time.Time) error }
	timeout time.Duration
}

// NewReadDeadlineTransport returns a Transport that sets a read deadline of
// the given timeout before every Read. If t does not support SetReadDeadline,
// t is returned unchanged.
func NewReadDeadlineTransport(t connection.Transport, timeout time.Duration) connection.Transport {
	ds, ok := t.(interface{ SetReadDeadline(time.Time) error })
	if !ok {
		return t
	}
	return &ReadDeadlineTransport{Transport: t, timeout: timeout, ds: ds}
}

func (d *ReadDeadlineTransport) Read(p []byte) (int, error) {
	_ = d.ds.SetReadDeadline(time.Now().Add(d.timeout))
	return d.Transport.Read(p)
}
