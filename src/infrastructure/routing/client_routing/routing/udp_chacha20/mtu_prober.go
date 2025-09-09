package udp_chacha20

import (
	"net/netip"
	"time"

	"tungo/application"
	"tungo/infrastructure/settings"

	"golang.org/x/crypto/chacha20poly1305"
)

// MTUProbeHandler implements application.MTUProber. It is a transport handler
// that sends in-band probes over the established connection and awaits
// acknowledgements.
//
// Buffer layout: [0..11] nonce | [12..] payload | [n+12..] tag
// The buffer is reused for all probes and responses.
type MTUProbeHandler struct {
	conn   application.ConnectionAdapter
	crypto application.CryptographyService
	buf    [settings.MTU + settings.UDPChacha20Overhead]byte
}

func NewMTUProbeHandler(conn application.ConnectionAdapter, crypto application.CryptographyService) application.MTUProber {
	return &MTUProbeHandler{conn: conn, crypto: crypto}
}

// SendProbe crafts an MTU probe of the requested size, encrypts it in place and
// writes it to the underlying connection via the handler.
func (p *MTUProbeHandler) SendProbe(size int) error {
	pkt := application.BuildMTUPacket(p.buf[chacha20poly1305.NonceSize:], application.MTUProbeType, size)
	enc, err := p.crypto.Encrypt(p.buf[:chacha20poly1305.NonceSize+len(pkt)])
	if err != nil {
		return err
	}
	_, err = p.conn.Write(enc)
	return err
}

// AwaitAck waits for an acknowledgement frame. A read deadline is set on the
// connection to prevent indefinite blocking. Timeouts are reported as a clean
// "no ack" result.
func (p *MTUProbeHandler) AwaitAck(timeout time.Duration) (bool, time.Duration, error) {
	if timeout > 0 {
		if rd, ok := p.conn.(interface{ SetReadDeadline(time.Time) error }); ok {
			if err := rd.SetReadDeadline(time.Now().Add(timeout)); err != nil {
				return false, 0, err
			}
			defer rd.SetReadDeadline(time.Time{})
		}
	}
	start := time.Now()
	n, err := p.conn.Read(p.buf[:])
	rtt := time.Since(start)
	if err != nil {
		if ne, ok := err.(interface{ Timeout() bool }); ok && ne.Timeout() {
			return false, rtt, nil
		}
		return false, rtt, err
	}
	dec, err := p.crypto.Decrypt(p.buf[:n])
	if err != nil {
		return false, rtt, err
	}
	if len(dec) <= 20 {
		return false, rtt, nil
	}
	dest := netip.AddrFrom4([4]byte{dec[16], dec[17], dec[18], dec[19]})
	if dest == application.ServiceIP && dec[20] == application.MTUAckType {
		return true, rtt, nil
	}
	return false, rtt, nil
}
