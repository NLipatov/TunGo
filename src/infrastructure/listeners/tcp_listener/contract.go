package tcp_listener

import "net"

type Listener interface {
	Accept() (net.Conn, error)
	Close() error
}
