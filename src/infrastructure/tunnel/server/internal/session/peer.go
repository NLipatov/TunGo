package session

import (
	"errors"
	"io"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"tungo/infrastructure/cryptography/primitives"
	outbound "tungo/infrastructure/tunnel/internal/egress"
)

var ErrPeerClosed = errors.New("peer closed")

type peerSender interface {
	Send([]byte) error
	Close() error
}

type crypto interface {
	Encrypt([]byte) ([]byte, error)
	Decrypt([]byte) ([]byte, error)
}

type peerRekey interface {
	Handle(uint16, primitives.KeyDeriver, []byte) ([]byte, uint16, bool, error)
	ObservePeerEpoch(uint16)
	ActivateSendEpoch(uint16)
}

// Peer is a session paired with its egress path — the unit stored in Repository.
//
// LIFECYCLE INVARIANT: The closed flag is set BEFORE zeroing crypto.
// Send and Decrypt hold cryptoMu from the closed check through the crypto
// operation, preventing concurrent zeroization.
type Peer struct {
	crypto       crypto
	rekey        peerRekey
	egress       peerSender
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
	crypto crypto,
	rekey peerRekey,
	internalIP netip.Addr,
	externalIP netip.AddrPort,
	writer io.Writer,
) *Peer {
	return NewPeerWithAuth(crypto, rekey, internalIP, externalIP, nil, nil, writer)
}

func NewPeerWithAuth(
	crypto crypto,
	rekey peerRekey,
	internalIP netip.Addr,
	externalIP netip.AddrPort,
	clientPubKey []byte,
	allowedIPs []netip.Prefix,
	writer io.Writer,
) *Peer {
	return newPeerWithAuth(
		crypto, rekey, internalIP, externalIP, clientPubKey, allowedIPs,
		outbound.New(writer, crypto),
	)
}

func newPeer(
	crypto crypto,
	rekey peerRekey,
	internalIP netip.Addr,
	externalIP netip.AddrPort,
	egress peerSender,
) *Peer {
	return newPeerWithAuth(crypto, rekey, internalIP, externalIP, nil, nil, egress)
}

func newPeerWithAuth(
	crypto crypto,
	rekey peerRekey,
	internalIP netip.Addr,
	externalIP netip.AddrPort,
	clientPubKey []byte,
	allowedIPs []netip.Prefix,
	egress peerSender,
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

func (p *Peer) HandleRekey(
	carrierEpoch uint16,
	deriver primitives.KeyDeriver,
	plaintext []byte,
) ([]byte, uint16, bool, error) {
	if p.rekey == nil {
		return nil, 0, false, nil
	}
	return p.rekey.Handle(carrierEpoch, deriver, plaintext)
}

func (p *Peer) ObservePeerEpoch(epoch uint16) {
	if p.rekey != nil {
		p.rekey.ObservePeerEpoch(epoch)
	}
}

func (p *Peer) ActivateSendEpoch(epoch uint16) {
	if p.rekey != nil {
		p.rekey.ActivateSendEpoch(epoch)
	}
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
