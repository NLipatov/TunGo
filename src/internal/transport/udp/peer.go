package udp

import (
	"io"
	"net/netip"
	"tungo/internal/config/settings"
)

type peerConn struct {
	conn     UdpListener
	addrPort netip.AddrPort

	readBuffer [settings.DefaultEthernetMTU + settings.UDPChacha20Overhead]byte
	oob        [8 * 1024]byte
}

func NewPeerConn(udpConn UdpListener, addrPort netip.AddrPort) io.ReadWriteCloser {
	return &peerConn{
		conn:     udpConn,
		addrPort: addrPort,
	}
}

func (ua *peerConn) Write(data []byte) (int, error) {
	return ua.conn.WriteToUDPAddrPort(data, ua.addrPort)
}

func (ua *peerConn) Read(buffer []byte) (int, error) {
	// Fast path: packet loops supply max-sized buffers; read directly and avoid copy.
	if len(buffer) >= len(ua.readBuffer) {
		n, _, _, _, err := ua.conn.ReadMsgUDPAddrPort(buffer[:len(ua.readBuffer)], ua.oob[:])
		if err != nil {
			return 0, err
		}
		return n, nil
	}

	n, _, _, _, err := ua.conn.ReadMsgUDPAddrPort(ua.readBuffer[:], ua.oob[:])
	if err != nil {
		return 0, err
	}
	if len(buffer) < n {
		copy(buffer, ua.readBuffer[:len(buffer)])
		return len(buffer), nil
	}
	copy(buffer, ua.readBuffer[:n])
	return n, nil
}

func (ua *peerConn) Close() error {
	return ua.conn.Close()
}
