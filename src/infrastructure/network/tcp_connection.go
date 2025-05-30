package network

import (
	"net"
	"tungo/application"
)

type DefaultConnection struct {
	socket application.Socket
}

func NewDefaultConnection(socket application.Socket) *DefaultConnection {
	return &DefaultConnection{
		socket: socket,
	}
}

func (u *DefaultConnection) Establish() (application.ConnectionAdapter, error) {
	dialer := net.Dialer{}
	conn, connErr := dialer.Dial("tcp", u.socket.StringAddr())
	if connErr != nil {
		return nil, connErr
	}

	return conn, nil
}
