package session_management

// ipv4Key consist of 4 octets describing IPv4 address
type ipv4Key [4]byte

type WorkerSessionManager[session ClientSession] interface {
	Add(session session)
	Delete(session session)
	GetByInternalIP(ip []byte) (session, error)
	GetByExternalIP(ip []byte) (session, error)
}

type DefaultWorkerSessionManager[CS ClientSession] struct {
	internalIpToSession map[ipv4Key]CS
	externalIPToSession map[ipv4Key]CS
}

func NewDefaultWorkerSessionManager[CS ClientSession]() WorkerSessionManager[CS] {
	return &DefaultWorkerSessionManager[CS]{
		internalIpToSession: make(map[ipv4Key]CS),
		externalIPToSession: make(map[ipv4Key]CS),
	}
}

func (s *DefaultWorkerSessionManager[CS]) Add(session CS) {
	s.internalIpToSession[ipv4Key(session.InternalIP())] = session
	s.externalIPToSession[ipv4Key(session.ExternalIP())] = session
}

func (s *DefaultWorkerSessionManager[CS]) Delete(session CS) {
	delete(s.internalIpToSession, ipv4Key(session.InternalIP()))
	delete(s.externalIPToSession, ipv4Key(session.ExternalIP()))
}

func (s *DefaultWorkerSessionManager[CS]) GetByInternalIP(ip []byte) (CS, error) {
	var zero CS
	if !s.validKeyLength(ip) {
		return zero, ErrInvalidIPLength
	}

	value, found := s.internalIpToSession[ipv4Key(ip)]
	if !found {
		return zero, ErrSessionNotFound
	}

	return value, nil
}

func (s *DefaultWorkerSessionManager[CS]) GetByExternalIP(ip []byte) (CS, error) {
	var zero CS
	if !s.validKeyLength(ip) {
		return zero, ErrInvalidIPLength
	}

	value, found := s.externalIPToSession[ipv4Key(ip)]
	if !found {
		return zero, ErrSessionNotFound
	}

	return value, nil
}

func (s *DefaultWorkerSessionManager[CS]) validKeyLength(key []byte) bool {
	// it's expected that IPv4 IP-address is exactly 4 bytes long
	return len(key) == 4
}
