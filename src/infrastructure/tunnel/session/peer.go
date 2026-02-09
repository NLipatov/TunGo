package session

import (
	"sync/atomic"
	"time"

	"tungo/application/network/connection"
)

// Peer is a session paired with its egress path — the unit stored in Repository.
//
// LIFECYCLE INVARIANT: The closed flag is set BEFORE zeroing crypto.
// Callers MUST check IsClosed() before using crypto to prevent use-after-free.
// This provides defense-in-depth against TOCTOU races in lookups.
type Peer struct {
	connection.Session
	egress       connection.Egress
	closed       atomic.Bool
	lastActivity atomic.Int64 // unix seconds
}

func NewPeer(session connection.Session, egress connection.Egress) *Peer {
	p := &Peer{Session: session, egress: egress}
	p.lastActivity.Store(time.Now().Unix())
	return p
}

func (p *Peer) Egress() connection.Egress {
	return p.egress
}

// IsClosed returns true if this peer has been marked for deletion.
// Callers MUST check this before using crypto to prevent use-after-free.
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

// SetLastActivityForTest overrides lastActivity (unix seconds) for testing.
// Must not be used in production code.
func (p *Peer) SetLastActivityForTest(unix int64) {
	p.lastActivity.Store(unix)
}

// markClosed sets the closed flag. Called by repository during Delete.
// After this returns, crypto will be zeroed; callers must not use it.
func (p *Peer) markClosed() {
	p.closed.Store(true)
}
