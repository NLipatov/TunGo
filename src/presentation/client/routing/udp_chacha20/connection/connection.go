package connection

import (
	"net"
	"tungo/settings"
)

type Connection interface {
	Establish() (*net.UDPConn, error)
}

type DefaultConnection struct {
	settings settings.ConnectionSettings
}

func NewConnection(settings settings.ConnectionSettings) *DefaultConnection {
	return &DefaultConnection{
		settings: settings,
	}
}

func (u *DefaultConnection) Establish() (*net.UDPConn, error) {
	serverAddr, serverAddrErr := u.resolveServerAddr()
	if serverAddrErr != nil {
		return nil, serverAddrErr
	}

	conn, err := net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func (u *DefaultConnection) resolveServerAddr() (*net.UDPAddr, error) {
	serverAddr := net.JoinHostPort(u.settings.ConnectionIP, u.settings.Port)
	udpAddr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		return nil, err
	}

	return udpAddr, nil
}
