package session

import "net/netip"

type Repository interface {
	// Add adds peer to the repository
	Add(peer *Peer)
	// Delete deletes peer from the repository
	Delete(peer *Peer)
	// GetByInternalAddrPort tries to retrieve peer by internal(in vpn) ip
	GetByInternalAddrPort(addr netip.Addr) (*Peer, error)
	// GetByExternalAddrPort tries to retrieve peer by external(outside of vpn) ip and port combination
	GetByExternalAddrPort(addrPort netip.AddrPort) (*Peer, error)
}

type DefaultRepository struct {
	internalIpToPeer map[netip.Addr]*Peer
	externalIPToPeer map[netip.AddrPort]*Peer
}

func NewDefaultRepository() Repository {
	return &DefaultRepository{
		internalIpToPeer: make(map[netip.Addr]*Peer),
		externalIPToPeer: make(map[netip.AddrPort]*Peer),
	}
}

func (s *DefaultRepository) Add(peer *Peer) {
	s.internalIpToPeer[peer.InternalAddr().Unmap()] = peer
	s.externalIPToPeer[s.canonicalAP(peer.ExternalAddrPort())] = peer
}

func (s *DefaultRepository) Delete(peer *Peer) {
	delete(s.internalIpToPeer, peer.InternalAddr().Unmap())
	delete(s.externalIPToPeer, s.canonicalAP(peer.ExternalAddrPort()))
}

func (s *DefaultRepository) GetByInternalAddrPort(addr netip.Addr) (*Peer, error) {
	value, found := s.internalIpToPeer[addr.Unmap()]
	if !found {
		return nil, ErrNotFound
	}
	return value, nil
}

func (s *DefaultRepository) GetByExternalAddrPort(addr netip.AddrPort) (*Peer, error) {
	value, found := s.externalIPToPeer[s.canonicalAP(addr)]
	if !found {
		return nil, ErrNotFound
	}
	return value, nil
}

func (s *DefaultRepository) canonicalAP(ap netip.AddrPort) netip.AddrPort {
	ip := ap.Addr().Unmap()
	return netip.AddrPortFrom(ip, ap.Port())
}
