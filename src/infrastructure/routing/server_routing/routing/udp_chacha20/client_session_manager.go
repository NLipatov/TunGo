package udp_chacha20

type UdpWorkerSessionManager struct {
	internalIpToSession map[[4]byte]ClientSession
	externalIPToSession map[[4]byte]ClientSession
}

func NewUdpWorkerSessionManager() UdpWorkerSessionManager {
	return UdpWorkerSessionManager{
		internalIpToSession: make(map[[4]byte]ClientSession),
		externalIPToSession: make(map[[4]byte]ClientSession),
	}
}

func (s *UdpWorkerSessionManager) Add(session ClientSession) {
	s.internalIpToSession[[4]byte(session.internalIP)] = session
	s.externalIPToSession[[4]byte(session.externalIP)] = session
}

func (s *UdpWorkerSessionManager) Delete(session ClientSession) {
	delete(s.externalIPToSession, [4]byte(session.externalIP))
	delete(s.internalIpToSession, [4]byte(session.internalIP))
}

func (s *UdpWorkerSessionManager) GetByInternalIP(ip []byte) (ClientSession, bool) {
	if len(ip) != 4 {
		return ClientSession{}, false
	}

	value, found := s.internalIpToSession[[4]byte(ip)]
	return value, found
}

func (s *UdpWorkerSessionManager) GetByExternalIP(ip []byte) (ClientSession, bool) {
	if len(ip) != 4 {
		return ClientSession{}, false
	}

	value, found := s.externalIPToSession[[4]byte(ip)]
	return value, found
}
