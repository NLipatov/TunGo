package adapters

import (
	"net/netip"
	"tungo/application/listeners"
	"tungo/application/network/connection"
	"tungo/infrastructure/settings"
)

type ServerUdpAdapter struct {
	conn     listeners.UdpListener
	addrPort netip.AddrPort

	readBuffer []byte
	oob        [8 * 1024]byte
}

func NewUdpAdapter(udpConn listeners.UdpListener, addrPort netip.AddrPort, mtu int) connection.Transport {
	return &ServerUdpAdapter{
		conn:       udpConn,
		addrPort:   addrPort,
		readBuffer: make([]byte, settings.UDPBufferSize(mtu)),
	}
}

func (ua *ServerUdpAdapter) Write(data []byte) (int, error) {
	return ua.conn.WriteToUDPAddrPort(data, ua.addrPort)
}

func (ua *ServerUdpAdapter) Read(buffer []byte) (int, error) {
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

func (ua *ServerUdpAdapter) Close() error {
	return ua.conn.Close()
}
