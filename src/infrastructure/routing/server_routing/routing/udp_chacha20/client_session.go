package udp_chacha20

import (
	"net"
	"tungo/application"
)

type clientSession struct {
	udpConn                *net.UDPConn
	udpAddr                *net.UDPAddr
	CryptographyService    application.CryptographyService
	internalIP, externalIP []byte
}
