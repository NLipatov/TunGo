package client_session

import (
	"net"
	"tungo/application"
)

type UdpSession struct {
	conn       *net.UDPConn
	internalIP string
	udpAddr    *net.UDPAddr
	session    application.CryptographyService
}

func NewUdpSession(conn *net.UDPConn, internalIP string, addr *net.UDPAddr, session application.CryptographyService) *UdpSession {
	return &UdpSession{
		conn:       conn,
		internalIP: internalIP,
		udpAddr:    addr,
		session:    session,
	}
}

func (s *UdpSession) Conn() *net.UDPConn {
	return s.conn
}

func (s *UdpSession) InternalIP() string {
	return s.internalIP
}

func (s *UdpSession) UdpAddr() *net.UDPAddr {
	return s.udpAddr
}

func (s *UdpSession) Session() application.CryptographyService {
	return s.session
}
