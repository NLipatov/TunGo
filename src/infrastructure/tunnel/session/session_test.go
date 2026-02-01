package session

import (
	"net/netip"
	"testing"
)

type sessionTestCrypto struct{}

func (d *sessionTestCrypto) Encrypt(b []byte) ([]byte, error) { return b, nil }
func (d *sessionTestCrypto) Decrypt(b []byte) ([]byte, error) { return b, nil }

func TestSessionAccessors(t *testing.T) {
	internal, _ := netip.ParseAddr("10.0.1.3")
	external, _ := netip.ParseAddrPort("93.184.216.34:9000")

	s := NewSession(
		&sessionTestCrypto{},
		nil,
		internal,
		external,
	)

	if got := s.InternalAddr(); got != internal {
		t.Errorf("InternalAddr() = %v, want %v", got, internal)
	}
	if got := s.ExternalAddrPort(); got != external {
		t.Errorf("ExternalAddrPort() = %v, want %v", got, external)
	}
	if s.Crypto() == nil {
		t.Error("Crypto() should not be nil")
	}
}
