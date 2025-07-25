package listeners

import "net"

type TcpListener interface {
	Accept() (net.Conn, error)
	Close() error
}
