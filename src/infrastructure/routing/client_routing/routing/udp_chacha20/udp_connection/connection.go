package udp_connection

import (
	"net"
	"tungo/application"
)

type Connection interface {
	Establish() (*net.UDPConn, error)
}

type DefaultConnection struct {
	socket application.Socket
}

func NewConnection(socket application.Socket) *DefaultConnection {
	return &DefaultConnection{
		socket: socket,
	}
}

func (u *DefaultConnection) Establish() (*net.UDPConn, error) {
	serverAddr, serverAddrErr := u.socket.UdpAddr()
	if serverAddrErr != nil {
		return nil, serverAddrErr
	}

	conn, err := net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		return nil, err
	}

	return conn, nil
}
