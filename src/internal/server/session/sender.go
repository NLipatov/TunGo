package session

import (
	"io"
	"net/netip"
	"sync"
)

type encryptedSender struct {
	writer io.Writer
	crypto interface {
		Encrypt([]byte) ([]byte, error)
	}
	mu sync.Mutex
}

func newEncryptedSender(writer io.Writer, crypto interface {
	Encrypt([]byte) ([]byte, error)
}) *encryptedSender {
	return &encryptedSender{writer: writer, crypto: crypto}
}

func (s *encryptedSender) Send(plaintext []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ciphertext, err := s.crypto.Encrypt(plaintext)
	if err != nil {
		return err
	}
	_, err = s.writer.Write(ciphertext)
	return err
}

func (s *encryptedSender) Close() error {
	if closer, ok := s.writer.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func (s *encryptedSender) SetAddrPort(addr netip.AddrPort) {
	if setter, ok := s.writer.(interface{ SetAddrPort(netip.AddrPort) }); ok {
		setter.SetAddrPort(addr)
	}
}
