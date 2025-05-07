package udp_chacha20

import "tungo/infrastructure/routing/server_routing/session_management"

type UdpWorkerSessionManager struct {
	internalIpToSession map[[4]byte]UdpSession
	externalIPToSession map[[4]byte]UdpSession
}

func NewUdpWorkerSessionManager() session_management.WorkerSessionManager[UdpSession] {
	return &UdpWorkerSessionManager{
		internalIpToSession: make(map[[4]byte]UdpSession),
		externalIPToSession: make(map[[4]byte]UdpSession),
	}
}

func (s *UdpWorkerSessionManager) Add(session UdpSession) {
	s.internalIpToSession[[4]byte(session.internalIP)] = session
	s.externalIPToSession[[4]byte(session.externalIP)] = session
}

func (s *UdpWorkerSessionManager) Delete(session UdpSession) {
	delete(s.externalIPToSession, [4]byte(session.externalIP))
	delete(s.internalIpToSession, [4]byte(session.internalIP))
}

func (s *UdpWorkerSessionManager) GetByInternalIP(ip []byte) (UdpSession, bool) {
	if len(ip) != 4 {
		return UdpSession{}, false
	}

	value, found := s.internalIpToSession[[4]byte(ip)]
	return value, found
}

func (s *UdpWorkerSessionManager) GetByExternalIP(ip []byte) (UdpSession, bool) {
	if len(ip) != 4 {
		return UdpSession{}, false
	}

	value, found := s.externalIPToSession[[4]byte(ip)]
	return value, found
}
