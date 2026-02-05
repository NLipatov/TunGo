package session

import (
	"sync/atomic"

	"tungo/application/network/connection"
)

// Peer is a session paired with its egress path — the unit stored in Repository.
//
// LIFECYCLE INVARIANT: The closed flag is set BEFORE zeroing crypto.
// Callers MUST check IsClosed() before using crypto to prevent use-after-free.
// This provides defense-in-depth against TOCTOU races in lookups.
type Peer struct {
	connection.Session
	egress connection.Egress
	closed atomic.Bool
}

func NewPeer(session connection.Session, egress connection.Egress) *Peer {
	return &Peer{Session: session, egress: egress}
}

func (p *Peer) Egress() connection.Egress {
	return p.egress
}

// IsClosed returns true if this peer has been marked for deletion.
// Callers MUST check this before using crypto to prevent use-after-free.
func (p *Peer) IsClosed() bool {
	return p.closed.Load()
}

// markClosed sets the closed flag. Called by repository during Delete.
// After this returns, crypto will be zeroed; callers must not use it.
func (p *Peer) markClosed() {
	p.closed.Store(true)
}
