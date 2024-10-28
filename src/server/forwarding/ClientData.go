package forwarding

import "net"

// ClientData is a entity which is ready to be sent to the client
type ClientData struct {
	conn  net.Conn
	extIP string //client's external ip
	data  []byte
}
