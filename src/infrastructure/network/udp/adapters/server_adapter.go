package adapters

import (
	"net/netip"
	"tungo/application/listeners"
	"tungo/application/network/connection"
	"tungo/infrastructure/settings"
)

type ServerAdapter struct {
	conn     listeners.UdpListener
	addrPort netip.AddrPort

	readBuffer [settings.DefaultEthernetMTU + settings.UDPChacha20Overhead]byte
	oob        [8 * 1024]byte
}

func NewServerAdapter(listener listeners.UdpListener, addrPort netip.AddrPort) connection.Transport {
	return &ServerAdapter{
		conn:     listener,
		addrPort: addrPort,
	}
}

func (a *ServerAdapter) Write(data []byte) (int, error) {
	return a.conn.WriteToUDPAddrPort(data, a.addrPort)
}

func (a *ServerAdapter) Read(buffer []byte) (int, error) {
	// Fast path: dataplane supplies max-sized buffers; read directly and avoid copy.
	if len(buffer) >= len(a.readBuffer) {
		n, _, _, _, err := a.conn.ReadMsgUDPAddrPort(buffer[:len(a.readBuffer)], a.oob[:])
		if err != nil {
			return 0, err
		}
		return n, nil
	}

	n, _, _, _, err := a.conn.ReadMsgUDPAddrPort(a.readBuffer[:], a.oob[:])
	if err != nil {
		return 0, err
	}
	if len(buffer) < n {
		copy(buffer, a.readBuffer[:len(buffer)])
		return len(buffer), nil
	}
	copy(buffer, a.readBuffer[:n])
	return n, nil
}

func (a *ServerAdapter) Close() error {
	return a.conn.Close()
}
