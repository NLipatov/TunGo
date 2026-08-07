package session

import (
	"errors"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"tungo/application/network/connection"
	"tungo/infrastructure/tunnel/controlplane"
)

var ErrPeerClosed = errors.New("peer closed")

// Peer is a session paired with its egress path — the unit stored in Repository.
//
// LIFECYCLE INVARIANT: The closed flag is set BEFORE zeroing crypto.
// Send and Decrypt hold cryptoMu from the closed check through the crypto
// operation, preventing concurrent zeroization.
type Peer struct {
	crypto       connection.Crypto
	rekey        *controlplane.ServerRekeyCoordinator
	egress       connection.Egress
	internalIP   netip.Addr
	externalIP   netip.AddrPort
	clientPubKey []byte
	allowedAddrs map[netip.Addr]struct{}
	allowedNets  []netip.Prefix
	closed       atomic.Bool
	lastActivity atomic.Int64 // unix seconds
	roamedAddr   atomic.Pointer[netip.AddrPort]
	cryptoMu     sync.RWMutex // protects crypto from concurrent zeroize
}

func NewPeer(
	crypto connection.Crypto,
	rekey *controlplane.ServerRekeyCoordinator,
	internalIP netip.Addr,
	externalIP netip.AddrPort,
	egress connection.Egress,
) *Peer {
	return NewPeerWithAuth(crypto, rekey, internalIP, externalIP, nil, nil, egress)
}

func NewPeerWithAuth(
	crypto connection.Crypto,
	rekey *controlplane.ServerRekeyCoordinator,
	internalIP netip.Addr,
	externalIP netip.AddrPort,
	clientPubKey []byte,
	allowedIPs []netip.Prefix,
	egress connection.Egress,
) *Peer {
	allowedAddrs := make(map[netip.Addr]struct{}, len(allowedIPs))
	allowedNets := make([]netip.Prefix, 0, len(allowedIPs))
	for _, prefix := range allowedIPs {
		prefix = netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits())
		if prefix.IsSingleIP() {
			allowedAddrs[prefix.Addr()] = struct{}{}
			continue
		}
		allowedNets = append(allowedNets, prefix)
	}

	p := &Peer{
		crypto:       crypto,
		rekey:        rekey,
		egress:       egress,
		internalIP:   internalIP.Unmap(),
		externalIP:   externalIP,
		clientPubKey: append([]byte(nil), clientPubKey...),
		allowedAddrs: allowedAddrs,
		allowedNets:  allowedNets,
	}
	p.lastActivity.Store(time.Now().Unix())
	return p
}

func (p *Peer) InternalAddr() netip.Addr {
	return p.internalIP
}

func (p *Peer) RekeyController() *controlplane.ServerRekeyCoordinator {
	return p.rekey
}

func (p *Peer) IsSourceAllowed(srcIP netip.Addr) bool {
	srcIP = srcIP.Unmap()
	if srcIP == p.internalIP {
		return true
	}
	if _, ok := p.allowedAddrs[srcIP]; ok {
		return true
	}
	for _, prefix := range p.allowedNets {
		if prefix.Contains(srcIP) {
			return true
		}
	}
	return false
}

// ExternalAddrPort returns the roamed address if set, otherwise the registration address.
func (p *Peer) ExternalAddrPort() netip.AddrPort {
	if addr := p.roamedAddr.Load(); addr != nil {
		return *addr
	}
	return p.externalIP
}

// SetExternalAddrPort atomically updates the external address after NAT roaming.
func (p *Peer) SetExternalAddrPort(addr netip.AddrPort) {
	p.roamedAddr.Store(&addr)
}

// IsClosed returns true if this peer has been marked for deletion.
func (p *Peer) IsClosed() bool {
	return p.closed.Load()
}

// TouchActivity records the current time as the last activity for this peer.
// Called after successful packet decryption (not on invalid/garbage packets).
func (p *Peer) TouchActivity() {
	p.lastActivity.Store(time.Now().Unix())
}

// LastActivity returns when data was last received from this peer.
func (p *Peer) LastActivity() time.Time {
	return time.Unix(p.lastActivity.Load(), 0)
}

// Decrypt decrypts data while protecting session crypto from concurrent
// lifecycle teardown and zeroization.
func (p *Peer) Decrypt(data []byte) ([]byte, error) {
	p.cryptoMu.RLock()
	defer p.cryptoMu.RUnlock()

	if p.closed.Load() {
		return nil, ErrPeerClosed
	}
	return p.crypto.Decrypt(data)
}

// Send encrypts and writes data while protecting session crypto from concurrent
// lifecycle teardown and zeroization.
func (p *Peer) Send(data []byte) error {
	p.cryptoMu.RLock()
	defer p.cryptoMu.RUnlock()

	if p.closed.Load() {
		return ErrPeerClosed
	}
	return p.egress.Send(data)
}

// updateEgressAddr updates the egress writer's destination address after NAT roaming.
// Called by repository during UpdateExternalAddr.
func (p *Peer) updateEgressAddr(addr netip.AddrPort) {
	type addrPortSetter interface {
		SetAddrPort(netip.AddrPort)
	}
	if u, ok := p.egress.(addrPortSetter); ok {
		u.SetAddrPort(addr)
	}
}

// markClosed sets the closed flag. Called by repository during Delete.
// After this returns, crypto will be zeroed; callers must not use it.
func (p *Peer) markClosed() {
	p.closed.Store(true)
}
