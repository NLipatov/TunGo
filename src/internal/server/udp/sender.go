package udp

import (
	"fmt"
	"io"
	"net/netip"
	"sync"

	"tungo/internal/server/session"
)

type peerSender struct {
	writer io.Writer
	crypto interface {
		Encrypt([]byte) ([]byte, error)
	}
	mu sync.Mutex
}

func newPeerSender(writer io.Writer, crypto interface {
	Encrypt([]byte) ([]byte, error)
}) *peerSender {
	return &peerSender{writer: writer, crypto: crypto}
}

func (s *peerSender) Send(plaintext []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ciphertext, err := s.crypto.Encrypt(plaintext)
	if err != nil {
		return fmt.Errorf("%w: %w", session.ErrEncryption, err)
	}
	if _, err = s.writer.Write(ciphertext); err != nil {
		return fmt.Errorf("%w: %w", session.ErrTransportWrite, err)
	}
	return nil
}

func (s *peerSender) Close() error {
	if closer, ok := s.writer.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func (s *peerSender) SetAddrPort(addr netip.AddrPort) {
	if setter, ok := s.writer.(interface{ SetAddrPort(netip.AddrPort) }); ok {
		setter.SetAddrPort(addr)
	}
}
