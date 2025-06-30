package session_management

import "net/netip"

type WorkerSessionManager[session ClientSession] interface {
	Add(session session)
	Delete(session session)
	GetByInternalIP(addr netip.Addr) (session, error)
	GetByExternalIP(addrPort netip.AddrPort) (session, error)
}

type DefaultWorkerSessionManager[cs ClientSession] struct {
	internalIpToSession map[netip.Addr]cs
	externalIPToSession map[netip.AddrPort]cs
}

func NewDefaultWorkerSessionManager[cs ClientSession]() WorkerSessionManager[cs] {
	return &DefaultWorkerSessionManager[cs]{
		internalIpToSession: make(map[netip.Addr]cs),
		externalIPToSession: make(map[netip.AddrPort]cs),
	}
}

func (s *DefaultWorkerSessionManager[cs]) Add(session cs) {
	s.internalIpToSession[session.InternalIP().Unmap()] = session
	s.externalIPToSession[s.canonicalAP(session.ExternalIP())] = session
}

func (s *DefaultWorkerSessionManager[cs]) Delete(session cs) {
	delete(s.internalIpToSession, session.InternalIP().Unmap())
	delete(s.externalIPToSession, s.canonicalAP(session.ExternalIP()))
}

func (s *DefaultWorkerSessionManager[cs]) GetByInternalIP(addr netip.Addr) (cs, error) {
	var zero cs

	value, found := s.internalIpToSession[addr.Unmap()]
	if !found {
		return zero, ErrSessionNotFound
	}

	return value, nil
}

func (s *DefaultWorkerSessionManager[cs]) GetByExternalIP(addr netip.AddrPort) (cs, error) {
	var zero cs

	value, found := s.externalIPToSession[s.canonicalAP(addr)]
	if !found {
		return zero, ErrSessionNotFound
	}

	return value, nil
}

func (s *DefaultWorkerSessionManager[cs]) canonicalAP(ap netip.AddrPort) netip.AddrPort {
	ip := ap.Addr().Unmap()
	return netip.AddrPortFrom(ip, ap.Port())
}
