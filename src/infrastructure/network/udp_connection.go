package network

import (
	"net"
	"tungo/application"
)

type UdpConnection struct {
	socket application.Socket
}

func NewUdpConnection(socket application.Socket) *UdpConnection {
	return &UdpConnection{
		socket: socket,
	}
}

func (u *UdpConnection) Establish() (application.ConnectionAdapter, error) {
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
