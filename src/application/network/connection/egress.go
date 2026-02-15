package connection

import (
	"io"
	"net/netip"
	"sync"
)

// Egress is a single, serialized egress path for a session.
//
// It prevents concurrent Encrypt calls on the same crypto instance (UDP/TCP sessions
// are not safe for concurrent Encrypt due to internal nonce/counter and scratch buffers),
// and also avoids concurrent writes to the underlying transport where ordering matters.
type Egress interface {
	// SendDataIP sends a data-plane (IP) packet over the encrypted transport.
	// The plaintext buffer is passed to Crypto.Encrypt as-is.
	SendDataIP(plaintext []byte) error
	// SendControl sends a control-plane packet (e.g. rekey) over the encrypted transport.
	// The plaintext buffer is passed to Crypto.Encrypt as-is.
	SendControl(plaintext []byte) error
	// Close tears down the underlying transport. Safe to call multiple times.
	Close() error
}

type DefaultEgress struct {
	w  io.Writer
	c  Crypto
	mu sync.Mutex
}

func NewDefaultEgress(w io.Writer, c Crypto) *DefaultEgress {
	return &DefaultEgress{w: w, c: c}
}

func (o *DefaultEgress) SendDataIP(plaintext []byte) error {
	return o.send(plaintext)
}

func (o *DefaultEgress) SendControl(plaintext []byte) error {
	return o.send(plaintext)
}

func (o *DefaultEgress) Close() error {
	if c, ok := o.w.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// SetAddrPort updates the destination address of the underlying writer,
// if it supports address updates (e.g. UDP RegistrationAdapter after NAT roaming).
func (o *DefaultEgress) SetAddrPort(addr netip.AddrPort) {
	type addrPortSetter interface {
		SetAddrPort(netip.AddrPort)
	}
	if u, ok := o.w.(addrPortSetter); ok {
		u.SetAddrPort(addr)
	}
}

func (o *DefaultEgress) send(plaintext []byte) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	enc, err := o.c.Encrypt(plaintext)
	if err != nil {
		return err
	}
	_, err = o.w.Write(enc)
	return err
}
