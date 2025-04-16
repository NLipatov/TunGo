package client_session

import (
	"net"
	"sync"
)

type Manager struct {
	m sync.Map
}

func NewManager() *Manager {
	return &Manager{}
}

func (u *Manager) Store(session Session[*net.UDPConn, *net.UDPAddr]) {
	u.m.Store(session.InternalIP(), session)
	u.m.Store(session.Addr().String(), session)
}

func (u *Manager) Load(ip string) (Session[*net.UDPConn, *net.UDPAddr], bool) {
	v, ok := u.m.Load(ip)
	if ok {
		return v.(Session[*net.UDPConn, *net.UDPAddr]), ok
	}

	return nil, false
}

func (u *Manager) Delete(ip string) {
	v, ok := u.m.Load(ip)
	if ok {
		session := v.(Session[*net.UDPConn, *net.UDPAddr])
		u.m.Delete(session.InternalIP())
		u.m.Delete(session.Addr().String())
	}
}
