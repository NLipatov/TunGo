package connection

import (
	"net"
	"tungo/infrastructure/settings"
)

type Handshake interface {
	Id() [32]byte
	KeyClientToServer() []byte
	KeyServerToClient() []byte
	ServerSideHandshake(transport Transport) (net.IP, error)
	ClientSideHandshake(transport Transport, settings settings.Settings) error
	PeerMTU() (int, bool)
}
