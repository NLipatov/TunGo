package packets

import "net"

type IPHeader interface {
	GetSourceIP() net.IP
	GetDestinationIP() net.IP
}
