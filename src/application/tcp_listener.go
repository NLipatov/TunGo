package application

import "net"

type TcpListener interface {
	Accept() (net.Conn, error)
	Close() error
}
