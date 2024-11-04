package forwarding

import "net"

// ClientData is a entity which is ready to be sent to the client
type ClientData struct {
	Conn  net.Conn
	ExtIP string //client's external ip
	Data  []byte
}
