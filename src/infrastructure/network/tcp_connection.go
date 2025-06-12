package network

import (
	"net"
	"tungo/application"
)

type TcpConnection struct {
	socket application.Socket
}

func NewTcpConnection(socket application.Socket) *TcpConnection {
	return &TcpConnection{
		socket: socket,
	}
}

func (u *TcpConnection) Establish() (application.ConnectionAdapter, error) {
	dialer := net.Dialer{}
	conn, connErr := dialer.Dial("tcp", u.socket.StringAddr())
	if connErr != nil {
		return nil, connErr
	}

	return conn, nil
}
