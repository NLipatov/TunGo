package application

import (
	"net/netip"
	"time"

	"tungo/infrastructure/settings"
)

// ConnMTUProber implements MTUProber using a ConnectionAdapter and
// CryptographyService. Probes are sent as in-band service frames and
// acknowledgements are awaited on the same connection.
type ConnMTUProber struct {
	conn   ConnectionAdapter
	crypto CryptographyService
}

// NewConnMTUProber creates a MTUProber bound to the given connection and
// cryptography service.
func NewConnMTUProber(c ConnectionAdapter, crypto CryptographyService) MTUProber {
	return &ConnMTUProber{conn: c, crypto: crypto}
}

// SendProbe transmits a probe frame of the requested size.
func (p *ConnMTUProber) SendProbe(size int) error {
	pkt := BuildMTUPacket(MTUProbeType, size)
	enc, err := p.crypto.Encrypt(pkt)
	if err != nil {
		return err
	}
	_, err = p.conn.Write(enc)
	return err
}

// AwaitAck waits for an acknowledgement. The timeout is best-effort and relies
// on the underlying connection's read deadlines.
func (p *ConnMTUProber) AwaitAck(timeout time.Duration) (bool, time.Duration, error) {
	buf := make([]byte, settings.MTU+settings.UDPChacha20Overhead)
	start := time.Now()
	n, err := p.conn.Read(buf)
	rtt := time.Since(start)
	if err != nil {
		return false, rtt, err
	}
	dec, err := p.crypto.Decrypt(buf[:n])
	if err != nil {
		return false, rtt, err
	}
	if len(dec) <= 20 {
		return false, rtt, nil
	}
	dest := netip.AddrFrom4([4]byte{dec[16], dec[17], dec[18], dec[19]})
	if dest == ServiceIP && dec[20] == MTUAckType {
		return true, rtt, nil
	}
	return false, rtt, nil
}
