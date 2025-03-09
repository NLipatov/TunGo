package ip

import "net"

type Header interface {
	GetSourceIP() net.IP
	GetDestinationIP() net.IP
}
