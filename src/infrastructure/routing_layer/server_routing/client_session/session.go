package client_session

import (
	"net"
	"tungo/application"
)

type Session[T net.Conn, A net.Addr] interface {
	Conn() *net.UDPConn
	InternalIP() string
	UdpAddr() *net.UDPAddr
	Session() application.CryptographyService
}
