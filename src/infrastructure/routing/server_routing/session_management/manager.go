package session_management

import "net/netip"

type WorkerSessionManager[session ClientSession] interface {
	Add(session session)
	Delete(session session)
	GetByInternalIP(ip netip.Addr) (session, error)
	GetByExternalIP(ip netip.Addr) (session, error)
}

type DefaultWorkerSessionManager[cs ClientSession] struct {
	internalIpToSession map[netip.Addr]cs
	externalIPToSession map[netip.Addr]cs
}

func NewDefaultWorkerSessionManager[cs ClientSession]() WorkerSessionManager[cs] {
	return &DefaultWorkerSessionManager[cs]{
		internalIpToSession: make(map[netip.Addr]cs),
		externalIPToSession: make(map[netip.Addr]cs),
	}
}

func (s *DefaultWorkerSessionManager[cs]) Add(session cs) {
	s.internalIpToSession[session.InternalIP().Unmap()] = session
	s.externalIPToSession[session.ExternalIP().Unmap()] = session
}

func (s *DefaultWorkerSessionManager[cs]) Delete(session cs) {
	delete(s.internalIpToSession, session.InternalIP().Unmap())
	delete(s.externalIPToSession, session.ExternalIP().Unmap())
}

func (s *DefaultWorkerSessionManager[cs]) GetByInternalIP(addr netip.Addr) (cs, error) {
	var zero cs

	value, found := s.internalIpToSession[addr.Unmap()]
	if !found {
		return zero, ErrSessionNotFound
	}

	return value, nil
}

func (s *DefaultWorkerSessionManager[cs]) GetByExternalIP(addr netip.Addr) (cs, error) {
	var zero cs

	value, found := s.externalIPToSession[addr.Unmap()]
	if !found {
		return zero, ErrSessionNotFound
	}

	return value, nil
}
