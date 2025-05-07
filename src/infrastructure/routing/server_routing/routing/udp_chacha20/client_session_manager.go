package udp_chacha20

import (
	"fmt"
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
	if len(ip) != 4 {
		return session{}, fmt.Errorf("invalid ipv4 length (got %d bytes, expected 4 bytes)", len(ip))
	}

	value, found := s.internalIpToSession[ipv4Key(ip)]
	if !found {
		return session{}, fmt.Errorf("session not found")
	}

	return value, nil
}

func (s *workerSessionManager) GetByExternalIP(ip []byte) (session, error) {
	if len(ip) != 4 {
		return session{}, fmt.Errorf("invalid ipv4 length (got %d bytes, expected 4 bytes)", len(ip))
	}

	value, found := s.externalIPToSession[ipv4Key(ip)]
	if !found {
		return session{}, fmt.Errorf("session not found")
	}

	return value, nil
}
