package repository

import (
	"net/netip"
	"tungo/application/network/connection"
)

type SessionRepository[session connection.Session] interface {
	// Add adds session to session manager
	Add(session session)
	// Delete deletes session from session manager
	Delete(session session)
	// GetByInternalAddrPort tries to retrieve client session by internal(in vpn) ip and port combination
	GetByInternalAddrPort(addr netip.Addr) (session, error)
	// GetByExternalAddrPort tries to retrieve client session by external(outside of vpn) ip and port combination
	GetByExternalAddrPort(addrPort netip.AddrPort) (session, error)
}

type DefaultSessionRepository[cs connection.Session] struct {
	internalIpToSession map[netip.Addr]cs
	externalIPToSession map[netip.AddrPort]cs
}

func NewDefaultWorkerSessionManager[cs connection.Session]() SessionRepository[cs] {
	return &DefaultSessionRepository[cs]{
		internalIpToSession: make(map[netip.Addr]cs),
		externalIPToSession: make(map[netip.AddrPort]cs),
	}
}

func (s *DefaultSessionRepository[cs]) Add(session cs) {
	s.internalIpToSession[session.InternalAddr().Unmap()] = session
	s.externalIPToSession[s.canonicalAP(session.ExternalAddrPort())] = session
}

func (s *DefaultSessionRepository[cs]) Delete(session cs) {
	delete(s.internalIpToSession, session.InternalAddr().Unmap())
	delete(s.externalIPToSession, s.canonicalAP(session.ExternalAddrPort()))
}

func (s *DefaultSessionRepository[cs]) GetByInternalAddrPort(addr netip.Addr) (cs, error) {
	var zero cs

	value, found := s.internalIpToSession[addr.Unmap()]
	if !found {
		return zero, ErrSessionNotFound
	}

	return value, nil
}

func (s *DefaultSessionRepository[cs]) GetByExternalAddrPort(addr netip.AddrPort) (cs, error) {
	var zero cs

	value, found := s.externalIPToSession[s.canonicalAP(addr)]
	if !found {
		return zero, ErrSessionNotFound
	}

	return value, nil
}

func (s *DefaultSessionRepository[cs]) canonicalAP(ap netip.AddrPort) netip.AddrPort {
	ip := ap.Addr().Unmap()
	return netip.AddrPortFrom(ip, ap.Port())
}
