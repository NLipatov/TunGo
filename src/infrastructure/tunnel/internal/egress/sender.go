package egress

import (
	"io"
	"net/netip"
	"sync"
)

type crypto interface {
	Encrypt([]byte) ([]byte, error)
}

// Sender serializes encryption and transport writes for one tunnel session.
type Sender struct {
	writer io.Writer
	crypto crypto
	mu     sync.Mutex
}

func New(writer io.Writer, crypto crypto) *Sender {
	return &Sender{writer: writer, crypto: crypto}
}

func (s *Sender) Send(plaintext []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ciphertext, err := s.crypto.Encrypt(plaintext)
	if err != nil {
		return err
	}
	_, err = s.writer.Write(ciphertext)
	return err
}

func (s *Sender) Close() error {
	if closer, ok := s.writer.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// SetAddrPort updates the underlying UDP destination after NAT roaming.
func (s *Sender) SetAddrPort(addr netip.AddrPort) {
	type addrPortSetter interface {
		SetAddrPort(netip.AddrPort)
	}
	if setter, ok := s.writer.(addrPortSetter); ok {
		setter.SetAddrPort(addr)
	}
}
