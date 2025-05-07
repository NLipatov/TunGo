package udp_chacha20

import (
	"tungo/infrastructure/routing/server_routing/session_management"
)

// ipv4Key consist of 4 octets describing IPv4 address
type ipv4Key [4]byte

type workerSessionManager struct {
	internalIpToSession map[ipv4Key]session
	externalIPToSession map[ipv4Key]session
}

func NewUdpWorkerSessionManager() session_management.WorkerSessionManager[session] {
	return &workerSessionManager{
		internalIpToSession: make(map[ipv4Key]session),
		externalIPToSession: make(map[ipv4Key]session),
	}
}

func (s *workerSessionManager) Add(session session) {
	s.internalIpToSession[ipv4Key(session.internalIP)] = session
	s.externalIPToSession[ipv4Key(session.externalIP)] = session
}

func (s *workerSessionManager) Delete(session session) {
	delete(s.externalIPToSession, ipv4Key(session.externalIP))
	delete(s.internalIpToSession, ipv4Key(session.internalIP))
}

func (s *workerSessionManager) GetByInternalIP(ip []byte) (session, error) {
	if !s.validKeyLength(ip) {
		return session{}, ErrInvalidIPLength
	}

	value, found := s.internalIpToSession[ipv4Key(ip)]
	if !found {
		return session{}, ErrSessionNotFound
	}

	return value, nil
}

func (s *workerSessionManager) GetByExternalIP(ip []byte) (session, error) {
	if !s.validKeyLength(ip) {
		return session{}, ErrInvalidIPLength
	}

	value, found := s.externalIPToSession[ipv4Key(ip)]
	if !found {
		return session{}, ErrSessionNotFound
	}

	return value, nil
}

func (s *workerSessionManager) validKeyLength(key []byte) bool {
	// it's expected that IPv4 IP-address is exactly 4 bytes long
	return len(key) == 4
}
