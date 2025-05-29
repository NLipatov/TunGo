package tcp_connection

import (
	"net"
	"tungo/application"
)

type Connection interface {
	Establish() (net.Conn, error)
}

type DefaultConnection struct {
	socket application.Socket
}

func NewDefaultConnection(socket application.Socket) *DefaultConnection {
	return &DefaultConnection{
		socket: socket,
	}
}

func (u *DefaultConnection) Establish() (net.Conn, error) {
	dialer := net.Dialer{}
	conn, connErr := dialer.Dial("tcp", u.socket.StringAddr())
	if connErr != nil {
		return nil, connErr
	}

	return conn, nil
}
