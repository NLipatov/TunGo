package udp_chacha20

import (
	"net"
	"tungo/settings"
)

type Connection interface {
	Establish() (*net.UDPConn, error)
}

type UDPConnection struct {
	settings settings.ConnectionSettings
}

func NewUDPConnection(settings settings.ConnectionSettings) *UDPConnection {
	return &UDPConnection{
		settings: settings,
	}
}

func (u *UDPConnection) Establish() (*net.UDPConn, error) {
	serverAddr := net.JoinHostPort(u.settings.ConnectionIP, u.settings.Port)
	udpAddr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		return nil, err
	}

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return nil, err
	}

	return conn, nil
}
