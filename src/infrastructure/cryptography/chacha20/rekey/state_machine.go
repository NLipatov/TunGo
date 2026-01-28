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

// StateMachine holds control-plane rekey state; crypto remains immutable and handshake-agnostic.
// It is intentionally not in the cryptography package to separate concerns.
type StateMachine struct {
	mu             sync.Mutex
	Crypto         Rekeyer
	IsServer       bool
	CurrentC2S     []byte
	CurrentS2C     []byte
	PendingPriv    *[32]byte
	LastRekeyEpoch uint16
	sendEpoch      uint16
	pendingSend    *uint16
	pendingSince   time.Time
	state          State
	pendingTimeout time.Duration
}

const maxEpochSafety = 65000

var (
	ErrEpochExhausted = fmt.Errorf("epoch exhausted; requires full re-handshake")
)

type State int

const (
	StateStable State = iota
	// StateInstalling means we started Rekey() but have not yet applied keys.
	StateInstalling
	// StatePending means new keys are installed for receive; send switch awaits confirmation.
	StatePending
)

// Rekey FSM (single in-flight rekey):
// States:
//   Stable: no pending epoch, sendEpoch active.
//   Installing: Rekey() in progress, keys not yet applied.
//   Pending: new epoch installed for recv, waiting for ConfirmSendEpoch (data) or timeout AbortPending.
//
// Allowed transitions:
//   Stable --RekeyAndApply--> Installing --applyKeysLocked--> Pending --ConfirmSendEpoch--> Stable
//   Pending --AbortPending/MaybeAbortPending(timeout)--> Stable
//
// Forbidden (must error/no-op):
//   RekeyAndApply when not Stable
//   ApplyKeys when not Stable or Installing
//   ConfirmSendEpoch when not Pending
//   second pending (pendingSend != nil) creation
//   transitions that would remove last/active epoch (enforced in Rekeyer.RemoveEpoch/Rekey guards)

func NewController(core Rekeyer, c2s, s2c []byte, isServer bool) *StateMachine {
	return &StateMachine{
		Crypto:         core,
		IsServer:       isServer,
		CurrentC2S:     append([]byte(nil), c2s...),
		CurrentS2C:     append([]byte(nil), s2c...),
		sendEpoch:      0,
		state:          StateStable,
		pendingTimeout: 5 * time.Second,
	}
}

// SetPendingTimeout overrides the timeout used to auto-abort pending rekeys.
// Primarily for tests; production should use default.
func (c *StateMachine) SetPendingTimeout(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pendingTimeout = d
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

// State returns the current FSM state.
func (c *StateMachine) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

// PendingEpoch returns the pending epoch, if any.
func (c *StateMachine) PendingEpoch() (uint16, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pendingSend == nil {
		return 0, false
	}
	return *c.pendingSend, true
}

// ApplyKeys updates the stored raw keys after a successful rekey operation.
// sendKey/recvKey follow the same orientation as the crypto: sendKey is for
// outbound traffic of this peer. epoch must be strictly increasing.
func (c *StateMachine) ApplyKeys(sendKey, recvKey []byte, epoch uint16) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state != StateInstalling && c.state != StateStable {
		return fmt.Errorf("invalid state for ApplyKeys: %v", c.state)
	}
	return c.applyKeysLocked(sendKey, recvKey, epoch)
}

// RekeyAndApply performs an atomic control-plane update:
// 1) ensures no rekey is already pending
// 2) asks crypto to install a new session
// 3) records the new keys and marks the epoch pending for send confirmation
// If any step fails, no control-plane state is mutated.
func (c *StateMachine) RekeyAndApply(sendKey, recvKey []byte) (uint16, error) {
	c.mu.Lock()
	if c.state != StateStable {
		curState := c.state
		c.mu.Unlock()
		return 0, fmt.Errorf("rekey not allowed in state %v", curState)
	}
	if c.LastRekeyEpoch >= maxEpochSafety {
		c.mu.Unlock()
		return 0, ErrEpochExhausted
	}
	c.state = StateInstalling
	c.mu.Unlock()

	epoch, err := c.Crypto.Rekey(sendKey, recvKey)
	if err != nil {
		c.mu.Lock()
		c.state = StateStable
		c.mu.Unlock()
		return 0, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state != StateInstalling {
		return 0, fmt.Errorf("unexpected state after rekey: %v", c.state)
	}
	if err := c.applyKeysLocked(sendKey, recvKey, epoch); err != nil {
		c.state = StateStable
		return 0, err
	}
	c.state = StatePending
	return epoch, nil
}

// applyKeysLocked assumes c.mu is held.
func (c *StateMachine) applyKeysLocked(sendKey, recvKey []byte, epoch uint16) error {
	if epoch <= c.LastRekeyEpoch {
		return fmt.Errorf("non-monotonic epoch: got %d, last %d", epoch, c.LastRekeyEpoch)
	}
	if c.IsServer {
		c.CurrentS2C = append([]byte(nil), sendKey...)
		c.CurrentC2S = append([]byte(nil), recvKey...)
	} else {
		c.CurrentC2S = append([]byte(nil), sendKey...)
		c.CurrentS2C = append([]byte(nil), recvKey...)
	}
	// new keys ready for receive; defer send switch until confirmation
	c.pendingSend = &epoch
	c.pendingSince = time.Now()
	return nil
}

// ConfirmSendEpoch promotes pending epoch to active for sending once a packet
// with the pending epoch is successfully received.
func (c *StateMachine) ConfirmSendEpoch(epoch uint16) {
	c.mu.Lock()
	if c.state != StatePending || c.pendingSend == nil || epoch != *c.pendingSend || epoch <= c.sendEpoch {
		c.mu.Unlock()
		return
	}
	old := c.sendEpoch
	c.Crypto.SetSendEpoch(epoch)
	c.sendEpoch = epoch
	c.pendingSend = nil
	c.LastRekeyEpoch = epoch
	c.state = StateStable
	c.mu.Unlock()

	// log outside lock to avoid blocking hot path
	fmt.Printf("send epoch switched: %d -> %d\n", old, epoch)
}

// AbortPending rolls back a pending rekey, removing the pending epoch session.
// It leaves the current send/recv session intact and returns to Stable.
func (c *StateMachine) AbortPending() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state != StatePending || c.pendingSend == nil {
		return
	}
	_ = c.Crypto.RemoveEpoch(*c.pendingSend) // best-effort
	c.pendingSend = nil
	// roll back epoch marker to the active send epoch to allow next rekey
	c.LastRekeyEpoch = c.sendEpoch
	c.state = StateStable
	fmt.Printf("send epoch abort pending; remain on %d\n", c.sendEpoch)
}

// MaybeAbortPending aborts if the pending timeout has elapsed.
func (c *StateMachine) MaybeAbortPending(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state != StatePending || c.pendingSend == nil {
		return
	}
	if now.Sub(c.pendingSince) >= c.pendingTimeout {
		_ = c.Crypto.RemoveEpoch(*c.pendingSend)
		c.pendingSend = nil
		c.state = StateStable
		fmt.Printf("send epoch abort by timeout; remain on %d\n", c.sendEpoch)
	}
}
