package application

import (
	"net"
	"tungo/infrastructure/settings"
)

type Handshake interface {
	Id() [32]byte
	ClientKey() []byte
	ServerKey() []byte
	ServerSideHandshake(conn ConnectionAdapter) (net.IP, error)
	ClientSideHandshake(conn ConnectionAdapter, settings settings.Settings) error
}
