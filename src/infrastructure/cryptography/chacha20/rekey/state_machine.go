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

type FSM interface {
	State() State
	StartRekey(sendKey, recvKey []byte) (uint16, error)
	ActivateSendEpoch(epoch uint16)
	AbortPendingIfExpired(now time.Time)
}

// StateMachine holds control-plane rekey state; crypto remains immutable and handshake-agnostic.
// It is intentionally not in the cryptography package to separate concerns.
type StateMachine struct {
	mu               sync.Mutex
	crypto           Rekeyer
	IsServer         bool
	CurrentC2S       []byte
	CurrentS2C       []byte
	PendingPriv      *[32]byte
	LastRekeyEpoch   uint16
	sendEpoch        uint16
	pendingSendEpoch uint16
	pendingSince     time.Time
	state            State
	pendingTimeout   time.Duration
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
//	Pending --AbortPending/AbortPendingIfExpired(timeout)--> Stable
//
// Forbidden (must error/no-op):
//
//	StartRekey when not Stable
//	ApplyKeys when not Stable or Rekeying
//	ActivateSendEpoch when not Pending
//	second pending (pendingSendEpoch != nil) creation
//	transitions that would remove last/active epoch (enforced in Rekeyer.RemoveEpoch/Rekey guards)

func NewStateMachine(core Rekeyer, c2s, s2c []byte, isServer bool) *StateMachine {
	return &StateMachine{
		crypto:         core,
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

// StartRekey performs an atomic control-plane update:
// 1) ensures no rekey is already pending
// 2) asks crypto to install a new session
// 3) records the new keys and marks the epoch pending for send confirmation
// If any step fails, no control-plane state is mutated.
func (c *StateMachine) StartRekey(sendKey, recvKey []byte) (uint16, error) {
	c.mu.Lock()
	if c.state != StateStable {
		c.mu.Unlock()
		return 0, fmt.Errorf("rekey not allowed in state %v", c.state)
	}
	if c.LastRekeyEpoch >= maxEpochSafety {
		c.mu.Unlock()
		return 0, ErrEpochExhausted
	}
	c.state = StateRekeying
	c.mu.Unlock() // Let FSM to process other events while wait for crypto
	epoch, err := c.crypto.Rekey(sendKey, recvKey)
	if err != nil {
		c.mu.Lock()
		c.state = StateStable // back to stable on crypto err
		c.mu.Unlock()
		return 0, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// is it still in StateRekeying?
	if c.state != StateRekeying {
		return 0, fmt.Errorf("unexpected state after rekey: %v", c.state)
	}
	// can we apply keys?
	if err := c.installPendingKeys(sendKey, recvKey, epoch); err != nil {
		c.state = StateStable
		return 0, err
	}
	// move next
	c.state = StatePending
	return epoch, nil
}

func (c *StateMachine) installPendingKeys(sendKey, recvKey []byte, epoch uint16) error {
	// epoch should monotonically increase
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
	c.pendingSendEpoch = epoch
	c.pendingSince = time.Now()
	return nil
}

// ActivateSendEpoch switches the local send side to the given epoch.
// It is called after a valid packet with this epoch is received from the peer,
// which proves the peer has installed the key and can decrypt traffic.
// After this call, outgoing packets are encrypted with the new epoch.
func (c *StateMachine) ActivateSendEpoch(epoch uint16) {
	c.mu.Lock()
	if c.state != StatePending || epoch != c.pendingSendEpoch || epoch <= c.sendEpoch {
		c.mu.Unlock()
		return
	}
	c.crypto.SetSendEpoch(epoch)
	c.sendEpoch = epoch
	c.LastRekeyEpoch = epoch
	c.state = StateStable
	c.mu.Unlock()
}

// AbortPendingIfExpired aborts if the pending timeout has elapsed.
func (c *StateMachine) AbortPendingIfExpired(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state != StatePending {
		return
	}
	if now.Sub(c.pendingSince) >= c.pendingTimeout {
		_ = c.crypto.RemoveEpoch(c.pendingSendEpoch)
		c.state = StateStable
		fmt.Printf("send epoch abort by timeout; remain on %d\n", c.sendEpoch)
	}
}
