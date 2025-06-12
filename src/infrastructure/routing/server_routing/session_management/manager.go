package session_management

// ipv4Key consist of 4 octets describing IPv4 address
type ipv4Key [4]byte

type WorkerSessionManager[session ClientSession] interface {
	Add(session session)
	Delete(session session)
	GetByInternalIP(ip []byte) (session, error)
	GetByExternalIP(ip []byte) (session, error)
}

type DefaultWorkerSessionManager[cs ClientSession] struct {
	internalIpToSession map[ipv4Key]cs
	externalIPToSession map[ipv4Key]cs
}

func NewDefaultWorkerSessionManager[cs ClientSession]() WorkerSessionManager[cs] {
	return &DefaultWorkerSessionManager[cs]{
		internalIpToSession: make(map[ipv4Key]cs),
		externalIPToSession: make(map[ipv4Key]cs),
	}
}

func (s *DefaultWorkerSessionManager[cs]) Add(session cs) {
	s.internalIpToSession[ipv4Key(session.InternalIP())] = session
	s.externalIPToSession[ipv4Key(session.ExternalIP())] = session
}

func (s *DefaultWorkerSessionManager[cs]) Delete(session cs) {
	delete(s.internalIpToSession, ipv4Key(session.InternalIP()))
	delete(s.externalIPToSession, ipv4Key(session.ExternalIP()))
}

func (s *DefaultWorkerSessionManager[cs]) GetByInternalIP(ip []byte) (cs, error) {
	var zero cs
	if !s.validKeyLength(ip) {
		return zero, ErrInvalidIPLength
	}

	value, found := s.internalIpToSession[ipv4Key(ip)]
	if !found {
		return zero, ErrSessionNotFound
	}

	return value, nil
}

func (s *DefaultWorkerSessionManager[cs]) GetByExternalIP(ip []byte) (cs, error) {
	var zero cs
	if !s.validKeyLength(ip) {
		return zero, ErrInvalidIPLength
	}

	value, found := s.externalIPToSession[ipv4Key(ip)]
	if !found {
		return zero, ErrSessionNotFound
	}

	return value, nil
}

func (s *DefaultWorkerSessionManager[cs]) validKeyLength(key []byte) bool {
	// it's expected that IPv4 IP-address is exactly 4 bytes long
	return len(key) == 4
}
