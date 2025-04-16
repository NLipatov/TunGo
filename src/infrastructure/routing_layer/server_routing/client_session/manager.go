package client_session

import (
	"net"
	"sync"
)

type Manager[Conn net.Conn, Addr net.Addr] struct {
	m sync.Map
}

func NewManager[Conn net.Conn, Addr net.Addr]() *Manager[Conn, Addr] {
	return &Manager[Conn, Addr]{}
}

func (u *Manager[Conn, Addr]) Store(session Session[*net.UDPConn, *net.UDPAddr]) {
	u.m.Store(session.InternalIP(), session)
	u.m.Store(session.Addr().String(), session)
}

func (u *Manager[Conn, Addr]) Load(ip string) (Session[*net.UDPConn, *net.UDPAddr], bool) {
	v, ok := u.m.Load(ip)
	if ok {
		return v.(Session[*net.UDPConn, *net.UDPAddr]), ok
	}

	return nil, false
}

func (u *Manager[Conn, Addr]) Delete(ip string) {
	v, ok := u.m.Load(ip)
	if ok {
		session := v.(Session[*net.UDPConn, *net.UDPAddr])
		u.m.Delete(session.InternalIP())
		u.m.Delete(session.Addr().String())
	}
}
