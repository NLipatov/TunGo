package udp_chacha20

import (
	"net/netip"
	"testing"
)

type sessionTestAdapter struct {
	closed bool
}

func (d *sessionTestAdapter) Write(_ []byte) (int, error) {
	return 0, nil
}

func (d *sessionTestAdapter) Read(_ []byte) (int, error) {
	return 0, nil
}

func (d *sessionTestAdapter) Close() error {
	d.closed = true
	return nil
}

type sessionTestCryptoService struct{}

func (d *sessionTestCryptoService) Encrypt(b []byte) ([]byte, error) { return b, nil }
func (d *sessionTestCryptoService) Decrypt(b []byte) ([]byte, error) { return b, nil }

func TestSessionAccessors(t *testing.T) {
	internal, _ := netip.ParseAddr("10.0.1.3")
	external, _ := netip.ParseAddrPort("93.184.216.34:9000")

	s := Session{
		connectionAdapter:   &sessionTestAdapter{},
		remoteAddrPort:      netip.AddrPort{},
		CryptographyService: &sessionTestCryptoService{},
		internalIP:          internal,
		externalIP:          external,
	}

	if got := s.InternalAddr(); got != internal {
		t.Errorf("InternalAddr() = %v, want %v", got, internal)
	}

	if got := s.ExternalAddrPort(); got != external {
		t.Errorf("ExternalAddrPort() = %v, want %v", got, external)
	}
}

func TestSession_Close(t *testing.T) {
	internal, _ := netip.ParseAddr("10.0.1.3")
	external, _ := netip.ParseAddrPort("93.184.216.34:9000")

	s := Session{
		connectionAdapter: &sessionTestAdapter{
			closed: false,
		},
		remoteAddrPort:      netip.AddrPort{},
		CryptographyService: &sessionTestCryptoService{},
		internalIP:          internal,
		externalIP:          external,
	}

	err := s.Close()
	if err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}
	if s.connectionAdapter.(*sessionTestAdapter).closed {
		t.Errorf("Close() closed connection, while it's expected to not. " +
			"(All udp clients are using this connection).")
	}
}
