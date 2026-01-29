package rekey

import (
	"fmt"
	"sync"
	"time"
)

// Rekeyer is the minimal interface the controller needs from the crypto layer.
// It returns the new epoch so callers can keep control-plane state consistent.
type Rekeyer interface {
	Rekey(sendKey, recvKey []byte) (uint16, error)
	SetSendEpoch(epoch uint16)
	RemoveEpoch(epoch uint16) bool
}

type State int

const (
	StateStable State = iota
	// StateRekeying means we started Rekey() but have not yet applied keys.
	StateRekeying
	// StatePending means new keys are installed for receive; send switch awaits confirmation.
	StatePending
)

type FSM interface {
	State() State
	StartRekey(sendKey, recvKey []byte) (uint16, error)
	ActivateSendEpoch(epoch uint16)
	AbortPendingIfExpired(now time.Time)
	CurrentServerToClientKey() []byte
	CurrentClientToServerKey() []byte
	IsServer() bool
}

// StateMachine holds control-plane rekey state; crypto remains immutable and handshake-agnostic.
// It is intentionally not in the cryptography package to separate concerns.
type StateMachine struct {
	mu     sync.Mutex
	crypto Rekeyer
	// now is injectable for tests; defaults to time.Now.
	now        func() time.Time
	isServer   bool
	CurrentC2S []byte
	CurrentS2C []byte
	// Pending key material is promoted to Current* only after ActivateSendEpoch confirms peer installed it.
	pendingC2S     []byte
	pendingS2C     []byte
	PendingPriv    *[32]byte
	LastRekeyEpoch uint16
	sendEpoch      uint16
	// Pending epoch bookkeeping.
	hasPending       bool
	pendingSendEpoch uint16
	pendingSince     time.Time
	state            State
	pendingTimeout   time.Duration
	// Highest epoch observed from peer on successfully processed traffic.
	// Needed to avoid losing "early ack" arriving while we're still in StateRekeying.
	peerEpochSeenMax uint16
}

const maxEpochSafety = 65000

var (
	ErrEpochExhausted = fmt.Errorf("epoch exhausted; requires full re-handshake")
)

// State Machine (single in-flight rekey):
// States:
//
//	Stable: no pending epoch, sendEpoch active.
//	Rekeying: Rekey() in progress, keys not yet applied.
//	Pending: new epoch installed for recv, waiting for ActivateSendEpoch (data) or timeout AbortPending.
//
// Allowed transitions:
//
//	Stable --StartRekey--> Rekeying --installPendingKeys--> Pending --ActivateSendEpoch--> Stable
//	Pending --AbortPendingIfExpired(timeout)--> Stable
//
// Forbidden (must error/no-op):
//
//	StartRekey when not Stable
//	ActivateSendEpoch does not transition unless we are Pending (but it always records peerEpochSeenMax)
//	second pending creation (hasPending == true)
//	transitions that would remove last/active epoch (enforced in Rekeyer.RemoveEpoch/Rekey guards)

func NewStateMachine(core Rekeyer, c2s, s2c []byte, isServer bool) *StateMachine {
	return &StateMachine{
		crypto:         core,
		now:            time.Now,
		isServer:       isServer,
		CurrentC2S:     append([]byte(nil), c2s...),
		CurrentS2C:     append([]byte(nil), s2c...),
		sendEpoch:      0,
		state:          StateStable,
		pendingTimeout: 5 * time.Second,
	}
}

// SetPendingTimeout overrides the timeout used to auto-abort pending rekeys.
// Primarily for tests; production should tune based on network conditions.
func (c *StateMachine) SetPendingTimeout(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pendingTimeout = d
}

// SetNowFunc injects a time source (useful for deterministic tests).
func (c *StateMachine) SetNowFunc(fn func() time.Time) {
	if fn == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = fn
}

func (c *StateMachine) SetPendingRekeyPrivateKey(priv [32]byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.PendingPriv = &priv
}

func (c *StateMachine) PendingRekeyPrivateKey() ([32]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.PendingPriv == nil {
		return [32]byte{}, false
	}
	return *c.PendingPriv, true
}

func (c *StateMachine) ClearPendingRekeyPrivateKey() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.PendingPriv = nil
}

func (c *StateMachine) CurrentClientToServerKey() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.CurrentC2S...)
}

func (c *StateMachine) CurrentServerToClientKey() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.CurrentS2C...)
}

func (c *StateMachine) IsServer() bool {
	return c.isServer
}

// State returns the current FSM state.
func (c *StateMachine) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

// StartRekey performs an atomic control-plane update:
// 1) ensures no rekey is already pending
// 2) asks crypto to install a new session
// 3) records the new keys and marks the epoch pending for send confirmation
// If any step fails, no control-plane state is mutated.
func (c *StateMachine) StartRekey(sendKey, recvKey []byte) (uint16, error) {
	var sendCopy, recvCopy []byte
	c.mu.Lock()
	if c.state != StateStable {
		c.mu.Unlock()
		return 0, fmt.Errorf("rekey not allowed in state %v", c.state)
	}
	if c.LastRekeyEpoch >= maxEpochSafety {
		c.mu.Unlock()
		return 0, ErrEpochExhausted
	}
	// Copy inputs before releasing the mutex to avoid races/external mutation of slices.
	sendCopy = append([]byte(nil), sendKey...)
	recvCopy = append([]byte(nil), recvKey...)
	c.state = StateRekeying
	c.mu.Unlock() // Allow FSM to process other events while waiting for crypto.
	epoch, err := c.crypto.Rekey(sendCopy, recvCopy)
	if err != nil {
		c.mu.Lock()
		c.state = StateStable
		c.mu.Unlock()
		return 0, err
	}
	c.mu.Lock()
	if c.state != StateRekeying {
		// Unexpected; avoid leaking epochs in crypto.
		prev := c.state
		c.state = StateStable
		c.mu.Unlock()
		_ = c.crypto.RemoveEpoch(epoch)
		return 0, fmt.Errorf("unexpected state after rekey: %v", prev)
	}
	// Also guard based on the epoch returned by crypto (LastRekeyEpoch is updated only on activation).
	if epoch >= maxEpochSafety {
		c.state = StateStable
		c.mu.Unlock()
		_ = c.crypto.RemoveEpoch(epoch)
		return 0, ErrEpochExhausted
	}
	if err := c.installPendingKeysLocked(sendCopy, recvCopy, epoch); err != nil {
		c.clearPendingLocked()
		c.state = StateStable
		c.mu.Unlock()
		_ = c.crypto.RemoveEpoch(epoch)
		return 0, err
	}
	c.state = StatePending
	// Handle "early ack": if peer's packet with this epoch arrived during StateRekeying,
	// ActivateSendEpoch may have been called already; try to activate now.
	c.maybeActivatePendingLocked()
	c.mu.Unlock()
	return epoch, nil
}

func (c *StateMachine) installPendingKeysLocked(sendKey, recvKey []byte, epoch uint16) error {
	// Only one pending rekey is allowed.
	if c.hasPending {
		return fmt.Errorf("pending rekey already exists")
	}
	// Epoch should monotonically increase across successful activations.
	if epoch <= c.LastRekeyEpoch || epoch <= c.sendEpoch {
		return fmt.Errorf("non-monotonic epoch: got %d, last %d", epoch, c.LastRekeyEpoch)
	}
	// Do not overwrite Current* until peer confirmation; keep pending separately.
	if c.isServer {
		c.pendingS2C = append([]byte(nil), sendKey...)
		c.pendingC2S = append([]byte(nil), recvKey...)
	} else {
		c.pendingC2S = append([]byte(nil), sendKey...)
		c.pendingS2C = append([]byte(nil), recvKey...)
	}
	c.hasPending = true
	c.pendingSendEpoch = epoch
	c.pendingSince = c.now()
	return nil
}

func (c *StateMachine) maybeActivatePendingLocked() {
	if c.state != StatePending {
		return
	}
	if !c.hasPending {
		return
	}
	if c.pendingSendEpoch <= c.sendEpoch {
		return
	}
	// Confirmed when we've observed any valid traffic from peer at >= pending epoch.
	if c.peerEpochSeenMax < c.pendingSendEpoch {
		return
	}
	epoch := c.pendingSendEpoch
	c.crypto.SetSendEpoch(epoch)
	c.sendEpoch = epoch
	c.LastRekeyEpoch = epoch
	// Promote pending keys to active.
	c.CurrentC2S = append([]byte(nil), c.pendingC2S...)
	c.CurrentS2C = append([]byte(nil), c.pendingS2C...)
	c.clearPendingLocked()
	c.state = StateStable
}

func (c *StateMachine) clearPendingLocked() {
	c.pendingC2S = nil
	c.pendingS2C = nil
	c.hasPending = false
	c.pendingSendEpoch = 0
	c.pendingSince = time.Time{}
}

// ActivateSendEpoch switches the local send side to the given epoch.
//
// IMPORTANT: This must be called only after a packet was successfully authenticated/decrypted
// using the key material for that epoch (i.e., the epoch confirmation must be cryptographically proven).
func (c *StateMachine) ActivateSendEpoch(epoch uint16) {
	c.mu.Lock()
	// Always record peer confirmation, even if we are not yet in StatePending.
	if epoch > c.peerEpochSeenMax {
		c.peerEpochSeenMax = epoch
	}
	// If we're pending, try to activate (covers both normal and early-ack cases).
	c.maybeActivatePendingLocked()
	c.mu.Unlock()
}

// AbortPendingIfExpired aborts if the pending timeout has elapsed.
func (c *StateMachine) AbortPendingIfExpired(now time.Time) {
	var (
		pendingEpoch uint16
		doAbort      bool
	)
	c.mu.Lock()
	if c.state != StatePending || !c.hasPending {
		c.mu.Unlock()
		return
	}
	if now.Sub(c.pendingSince) >= c.pendingTimeout {
		pendingEpoch = c.pendingSendEpoch
		c.clearPendingLocked()
		c.state = StateStable
		doAbort = true
	}
	c.mu.Unlock()

	if !doAbort {
		return
	}
	_ = c.crypto.RemoveEpoch(pendingEpoch)
}
