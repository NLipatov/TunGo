package udp_chacha20

import "tungo/infrastructure/routing/server_routing/session_management"

type workerSessionManager struct {
	internalIpToSession map[[4]byte]session
	externalIPToSession map[[4]byte]session
}

func NewUdpWorkerSessionManager() session_management.WorkerSessionManager[session] {
	return &workerSessionManager{
		internalIpToSession: make(map[[4]byte]session),
		externalIPToSession: make(map[[4]byte]session),
	}
}

func (s *workerSessionManager) Add(session session) {
	s.internalIpToSession[[4]byte(session.internalIP)] = session
	s.externalIPToSession[[4]byte(session.externalIP)] = session
}

func (s *workerSessionManager) Delete(session session) {
	delete(s.externalIPToSession, [4]byte(session.externalIP))
	delete(s.internalIpToSession, [4]byte(session.internalIP))
}

func (s *workerSessionManager) GetByInternalIP(ip []byte) (session, bool) {
	if len(ip) != 4 {
		return session{}, false
	}

	value, found := s.internalIpToSession[[4]byte(ip)]
	return value, found
}

func (s *workerSessionManager) GetByExternalIP(ip []byte) (session, bool) {
	if len(ip) != 4 {
		return session{}, false
	}

	value, found := s.externalIPToSession[[4]byte(ip)]
	return value, found
}
