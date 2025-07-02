package tcp_chacha20

import (
	"net"
	"net/netip"
	"testing"
	"time"
)

type sessionTestConn struct{}

func (d *sessionTestConn) Read([]byte) (int, error)           { return 0, nil }
func (d *sessionTestConn) Write([]byte) (int, error)          { return 0, nil }
func (d *sessionTestConn) Close() error                       { return nil }
func (d *sessionTestConn) LocalAddr() net.Addr                { return nil }
func (d *sessionTestConn) RemoteAddr() net.Addr               { return nil }
func (d *sessionTestConn) SetDeadline(_ time.Time) error      { return nil }
func (d *sessionTestConn) SetReadDeadline(_ time.Time) error  { return nil }
func (d *sessionTestConn) SetWriteDeadline(_ time.Time) error { return nil }

type sessionTestCrypto struct{}

func (d *sessionTestCrypto) Encrypt(b []byte) ([]byte, error) { return b, nil }
func (d *sessionTestCrypto) Decrypt(b []byte) ([]byte, error) { return b, nil }

func TestSessionAccessors(t *testing.T) {
	internal, _ := netip.ParseAddr("10.0.1.3")
	external, _ := netip.ParseAddrPort("93.184.216.34:9000")

	s := Session{
		conn:                &sessionTestConn{},
		CryptographyService: &sessionTestCrypto{},
		internalIP:          internal,
		externalIP:          external,
	}

	if got := s.InternalIP(); got != internal {
		t.Errorf("InternalIP() = %v, want %v", got, internal)
	}
	if got := s.ExternalIP(); got != external {
		t.Errorf("ExternalIP() = %v, want %v", got, external)
	}
}
