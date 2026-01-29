package connection

import (
	"io"
	"sync"
)

// Outbound is a single, serialized egress path for a session.
//
// It prevents concurrent Encrypt calls on the same crypto instance (UDP/TCP sessions
// are not safe for concurrent Encrypt due to internal nonce/counter and scratch buffers),
// and also avoids concurrent writes to the underlying transport where ordering matters.
type Outbound interface {
	// SendDataIP sends a data-plane (IP) packet over the encrypted transport.
	// The plaintext buffer is passed to Crypto.Encrypt as-is.
	SendDataIP(plaintext []byte) error
	// SendControl sends a control-plane packet (e.g. rekey) over the encrypted transport.
	// The plaintext buffer is passed to Crypto.Encrypt as-is.
	SendControl(plaintext []byte) error
}

type DefaultOutbound struct {
	w  io.Writer
	c  Crypto
	mu sync.Mutex
}

func NewDefaultOutbound(w io.Writer, c Crypto) *DefaultOutbound {
	return &DefaultOutbound{w: w, c: c}
}

func (o *DefaultOutbound) SendDataIP(plaintext []byte) error {
	return o.send(plaintext)
}

func (o *DefaultOutbound) SendControl(plaintext []byte) error {
	return o.send(plaintext)
}

func (o *DefaultOutbound) send(plaintext []byte) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	enc, err := o.c.Encrypt(plaintext)
	if err != nil {
		return err
	}
	_, err = o.w.Write(enc)
	return err
}
