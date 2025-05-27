package tcp_connection

import (
	"net"
	"tungo/infrastructure/settings"
)

type Connection interface {
	Establish() (net.Conn, error)
}

type DefaultConnection struct {
	settings settings.Settings
}

func NewDefaultConnection(settings settings.Settings) *DefaultConnection {
	return &DefaultConnection{
		settings: settings,
	}
}

func (u *DefaultConnection) Establish() (net.Conn, error) {
	dialer := net.Dialer{}
	conn, connErr := dialer.Dial("tcp", net.JoinHostPort(u.settings.ConnectionIP, u.settings.Port))
	if connErr != nil {
		return nil, connErr
	}

	return conn, nil
}
