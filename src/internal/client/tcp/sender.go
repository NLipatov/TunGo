package tcp

import (
	"io"
	"sync"
)

type packetSender struct {
	writer io.Writer
	crypto interface {
		Encrypt([]byte) ([]byte, error)
	}
	mu sync.Mutex
}

func newPacketSender(writer io.Writer, crypto interface {
	Encrypt([]byte) ([]byte, error)
}) *packetSender {
	return &packetSender{writer: writer, crypto: crypto}
}

func (s *packetSender) Send(plaintext []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ciphertext, err := s.crypto.Encrypt(plaintext)
	if err != nil {
		return err
	}
	_, err = s.writer.Write(ciphertext)
	return err
}
