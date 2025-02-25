package connection

import (
	"net"
	"tungo/settings"
)

type Connection interface {
	Establish() (*net.Conn, error)
}

type DefaultConnection struct {
	settings settings.ConnectionSettings
}

func NewDefaultConnection(settings settings.ConnectionSettings) *DefaultConnection {
	return &DefaultConnection{
		settings: settings,
	}
}

func (u *DefaultConnection) Establish() (*net.Conn, error) {
	dialer := net.Dialer{}
	conn, connErr := dialer.Dial("tcp", net.JoinHostPort(u.settings.ConnectionIP, u.settings.Port))
	if connErr != nil {
		return nil, connErr
	}

	return &conn, nil
}
