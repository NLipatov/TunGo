package tun_udp_chacha20

import "net"

type clientData struct {
	conn *net.UDPConn
	addr *net.UDPAddr
}
