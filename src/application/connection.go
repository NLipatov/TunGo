package application

import "net"

type Connection[T net.Conn] interface {
	Establish() (T, error)
}
