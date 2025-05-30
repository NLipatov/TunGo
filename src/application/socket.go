package application

import "net"

type Socket interface {
	StringAddr() string
	UdpAddr() (*net.UDPAddr, error)
}
