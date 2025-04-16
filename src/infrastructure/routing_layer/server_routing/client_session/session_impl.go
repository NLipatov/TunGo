package client_session

import (
	"net"
	"tungo/application"
)

type SessionImpl[Conn net.Conn, Addr net.Addr] struct {
	conn       Conn
	internalIP string
	udpAddr    Addr
	session    application.CryptographyService
}

func NewSessionImpl[Conn net.Conn, Addr net.Addr](
	conn Conn, internalIP string, addr Addr, session application.CryptographyService,
) Session[Conn, Addr] {
	return &SessionImpl[Conn, Addr]{
		conn:       conn,
		internalIP: internalIP,
		udpAddr:    addr,
		session:    session,
	}
}

func (s *SessionImpl[Conn, Addr]) Conn() Conn {
	return s.conn
}

func (s *SessionImpl[Conn, Addr]) InternalIP() string {
	return s.internalIP
}

func (s *SessionImpl[Conn, Addr]) Addr() Addr {
	return s.udpAddr
}

func (s *SessionImpl[Conn, Addr]) Session() application.CryptographyService {
	return s.session
}
