package client_session

import (
	"sync"
)

type UdpSessionManager struct {
	m sync.Map
}

func NewUdpSessionManager() *UdpSessionManager {
	return &UdpSessionManager{}
}

func (u *UdpSessionManager) Store(session *UdpSession) {
	u.m.Store(session.internalIP, session)
	u.m.Store(session.udpAddr.String(), session)
}

func (u *UdpSessionManager) Load(ip string) (*UdpSession, bool) {
	v, ok := u.m.Load(ip)
	if ok {
		return v.(*UdpSession), ok
	}

	return nil, false
}

func (u *UdpSessionManager) Delete(ip string) {
	v, ok := u.m.Load(ip)
	if ok {
		session := v.(*UdpSession)
		u.m.Delete(session.internalIP)
		u.m.Delete(session.udpAddr)
	}
}
