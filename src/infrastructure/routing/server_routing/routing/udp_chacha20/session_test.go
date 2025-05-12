package udp_chacha20

import (
	"bytes"
	"net/netip"
	"testing"
)

type sessionTestAdapter struct{}

func (d *sessionTestAdapter) Read([]byte) (int, error)  { return 0, nil }
func (d *sessionTestAdapter) Write([]byte) (int, error) { return 0, nil }
func (d *sessionTestAdapter) Close() error              { return nil }

type sessionTestCryptoService struct{}

func (d *sessionTestCryptoService) Encrypt(b []byte) ([]byte, error) { return b, nil }
func (d *sessionTestCryptoService) Decrypt(b []byte) ([]byte, error) { return b, nil }

func TestSessionAccessors(t *testing.T) {
	internal := []byte{10, 0, 1, 3}
	external := []byte{93, 184, 216, 34}

	s := session{
		connectionAdapter:   &sessionTestAdapter{},
		remoteAddrPort:      netip.AddrPort{},
		CryptographyService: &sessionTestCryptoService{},
		internalIP:          internal,
		externalIP:          external,
	}

	if got := s.InternalIP(); !bytes.Equal(got, internal) {
		t.Errorf("InternalIP() = %v, want %v", got, internal)
	}

	if got := s.ExternalIP(); !bytes.Equal(got, external) {
		t.Errorf("ExternalIP() = %v, want %v", got, external)
	}
}
