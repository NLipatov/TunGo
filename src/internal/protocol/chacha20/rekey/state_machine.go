package rekey

import (
	"fmt"
	"sync"
	"tungo/internal/protocol/securemem"

	"golang.org/x/crypto/chacha20poly1305"
)

// EpochManager owns the lifecycle of transport crypto epochs.
// It returns the new epoch so callers can keep control-plane state consistent.
type EpochManager interface {
	// StageEpoch reads canonical-direction keys only for the duration of the call
	// and returns a newly allocated, monotonically increasing epoch.
	StageEpoch(c2s, s2c []byte) (uint16, error)
	// PromoteSendEpoch switches outgoing traffic to the previously staged epoch.
	PromoteSendEpoch(epoch uint16)
	// RetirePreviousEpoch releases the previous epoch according to transport policy.
	RetirePreviousEpoch() bool
}

type keys struct {
	c2s   []byte
	s2c   []byte
	epoch uint16
}

type transitionPhase uint8

const (
	phaseIdle transitionPhase = iota
	phaseStaged
	phaseRetiring
)

type state struct {
	current              keys
	staged               keys
	maxObservedPeerEpoch uint16
	phase                transitionPhase
}

// StateMachine holds canonical control-plane keys and coordinates epoch lifecycle.
type StateMachine struct {
	mu           sync.Mutex
	epochManager EpochManager
	state        state
}

func NewStateMachine(epochManager EpochManager, c2s, s2c []byte) *StateMachine {
	return &StateMachine{
		epochManager: epochManager,
		state: state{
			current: keys{
				c2s: cloneKey(c2s),
				s2c: cloneKey(s2c),
			},
			staged: keys{
				c2s: make([]byte, chacha20poly1305.KeySize),
				s2c: make([]byte, chacha20poly1305.KeySize),
			},
		},
	}
}

func cloneKey(key []byte) []byte {
	capacity := max(len(key), chacha20poly1305.KeySize)
	cloned := make([]byte, len(key), capacity)
	copy(cloned, key)
	return cloned
}

// CurrentKeys returns caller-owned snapshots of both directional keys.
func (c *StateMachine) CurrentKeys() (clientToServer, serverToClient []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.state.current.c2s...), append([]byte(nil), c.state.current.s2c...)
}

func (c *StateMachine) ReadyForRekey() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state.phase == phaseIdle
}

// SendEpoch returns the epoch currently used for outbound packets. Control-plane
// transactions use it to bind a response to the epoch that carried its request.
func (c *StateMachine) SendEpoch() uint16 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state.current.epoch
}

// StartRekey performs an atomic control-plane update:
// 1) ensures no epoch is already staged
// 2) asks crypto to install a new session
// 3) records the staged keys and epoch for send activation
// If any step fails, no control-plane state is mutated.
func (c *StateMachine) StartRekey(c2s, s2c []byte) (uint16, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c2s) != chacha20poly1305.KeySize || len(s2c) != chacha20poly1305.KeySize {
		return 0, fmt.Errorf(
			"invalid rekey key size: c2s=%d s2c=%d want=%d",
			len(c2s),
			len(s2c),
			chacha20poly1305.KeySize,
		)
	}
	if c.state.phase != phaseIdle {
		return 0, fmt.Errorf("rekey already in progress")
	}
	staged := &c.state.staged
	epoch, err := c.epochManager.StageEpoch(c2s, s2c)
	if err != nil {
		return 0, err
	}
	copy(staged.c2s, c2s)
	copy(staged.s2c, s2c)
	staged.epoch = epoch
	c.state.phase = phaseStaged
	return epoch, nil
}

// ActivateSendEpoch commits the staged epoch for outbound encryption.
// The caller owns the protocol decision to activate; authenticated inbound
// observations are reported separately through ObservePeerEpoch.
func (c *StateMachine) ActivateSendEpoch(epoch uint16) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.activateStagedLocked(epoch) {
		return
	}
	c.epochManager.PromoteSendEpoch(epoch)
	c.retirePreviousEpochLocked()
}

func (c *StateMachine) retirePreviousEpochLocked() {
	state := &c.state
	if state.phase != phaseRetiring || state.maxObservedPeerEpoch < state.current.epoch {
		return
	}
	if c.epochManager.RetirePreviousEpoch() {
		state.phase = phaseIdle
	}
}

// ObservePeerEpoch records an epoch only after its packet was successfully
// authenticated. Once local send and peer receive have both moved forward,
// the previous epoch can be retired by the crypto implementation.
func (c *StateMachine) ObservePeerEpoch(epoch uint16) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state.maxObservedPeerEpoch = max(c.state.maxObservedPeerEpoch, epoch)
	c.retirePreviousEpochLocked()
}

func (c *StateMachine) activateStagedLocked(epoch uint16) bool {
	if c.state.phase != phaseStaged || epoch != c.state.staged.epoch {
		return false
	}
	c.state.current, c.state.staged = c.state.staged, c.state.current
	c.clearStagedLocked()
	c.state.phase = phaseRetiring
	return true
}

func (c *StateMachine) clearStagedLocked() {
	staged := &c.state.staged
	staged.c2s = staged.c2s[:chacha20poly1305.KeySize]
	staged.s2c = staged.s2c[:chacha20poly1305.KeySize]
	securemem.ZeroBytes(staged.c2s)
	securemem.ZeroBytes(staged.s2c)
}
