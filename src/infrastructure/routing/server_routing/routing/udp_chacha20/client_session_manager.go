package udp_chacha20

type sessionManager struct {
	internalIpToSession map[[4]byte]clientSession
	externalIPToSession map[[4]byte]clientSession
}

func newSessionManager() sessionManager {
	return sessionManager{
		internalIpToSession: make(map[[4]byte]clientSession),
		externalIPToSession: make(map[[4]byte]clientSession),
	}
}

func (s *sessionManager) add(session clientSession) {
	s.internalIpToSession[[4]byte(session.internalIP)] = session
	s.externalIPToSession[[4]byte(session.externalIP)] = session
}

func (s *sessionManager) delete(session clientSession) {
	delete(s.externalIPToSession, [4]byte(session.externalIP))
	delete(s.internalIpToSession, [4]byte(session.internalIP))
}

func (s *sessionManager) getByInternalIP(ip []byte) (clientSession, bool) {
	if len(ip) != 4 {
		return clientSession{}, false
	}

	value, found := s.internalIpToSession[[4]byte(ip)]
	return value, found
}

func (s *sessionManager) getByExternalIP(ip []byte) (clientSession, bool) {
	if len(ip) != 4 {
		return clientSession{}, false
	}

	value, found := s.externalIPToSession[[4]byte(ip)]
	return value, found
}
