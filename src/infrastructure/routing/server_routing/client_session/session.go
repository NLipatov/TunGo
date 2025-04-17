package client_session

import (
	"net"
	"tungo/application"
)

type Session[Conn net.Conn, Addr net.Addr] interface {
	Conn() Conn
	InternalIP() string
	Addr() Addr
	Session() application.CryptographyService
}
