package tun_tcp_chacha20

import "net"

// clientData is an entity which is ready to be sent to the client
type clientData struct {
	Conn  net.Conn
	ExtIP string //client's external ip
	Data  []byte
}
