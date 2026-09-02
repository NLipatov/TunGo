package rekey

import (
	"fmt"
	"sync"
	"sync/atomic"
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
	epoch atomic.Uint32
}

const (
	phaseIdle uint32 = iota
	phaseStaged
	phaseRetiring
)

type state struct {
	current              keys
	staged               keys
	maxObservedPeerEpoch atomic.Uint32
	phase                atomic.Uint32
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
	return c.state.phase.Load() == phaseIdle
}

// SendEpoch returns the epoch currently used for outbound packets. Control-plane
// transactions use it to bind a response to the epoch that carried its request.
func (c *StateMachine) SendEpoch() uint16 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return uint16(c.state.current.epoch.Load())
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
	if c.state.phase.Load() != phaseIdle {
		return 0, fmt.Errorf("rekey already in progress")
	}
	staged := &c.state.staged
	epoch, err := c.epochManager.StageEpoch(c2s, s2c)
	if err != nil {
		return 0, err
	}
	copy(staged.c2s, c2s)
	copy(staged.s2c, s2c)
	staged.epoch.Store(uint32(epoch))
	c.state.phase.Store(phaseStaged)
	return epoch, nil
}

// ActivateSendEpoch commits the staged epoch for outbound encryption.
// The caller owns the protocol decision to activate; authenticated inbound
// observations are reported separately through ObservePeerEpoch.
func (c *StateMachine) ActivateSendEpoch(epoch uint16) {
	if uint16(c.state.current.epoch.Load()) >= epoch {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.activateStagedLocked(epoch) {
		return
	}
	c.retirePreviousEpochLocked()
}

func (c *StateMachine) retirePreviousEpochLocked() {
	state := &c.state
	if state.phase.Load() != phaseRetiring || state.maxObservedPeerEpoch.Load() < state.current.epoch.Load() {
		return
	}
	if c.epochManager.RetirePreviousEpoch() {
		state.phase.Store(phaseIdle)
	}
}

// ObservePeerEpoch records an epoch only after its packet was successfully
// authenticated. Once local send and peer receive have both moved forward,
// the previous epoch can be retired by the crypto implementation.
func (c *StateMachine) ObservePeerEpoch(epoch uint16) {
	u32Epoch := uint32(epoch)
	if c.state.maxObservedPeerEpoch.Load() >= u32Epoch &&
		c.state.phase.Load() != phaseRetiring {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state.maxObservedPeerEpoch.Load() < u32Epoch {
		c.state.maxObservedPeerEpoch.Store(u32Epoch)
	}
	c.retirePreviousEpochLocked()
}

func (c *StateMachine) activateStagedLocked(epoch uint16) bool {
	if c.state.phase.Load() != phaseStaged || epoch != uint16(c.state.staged.epoch.Load()) {
		return false
	}
	c.epochManager.PromoteSendEpoch(epoch)
	stagedEpoch := c.state.staged.epoch.Load()
	oldCurrentEpoch := c.state.current.epoch.Swap(stagedEpoch)
	c.state.staged.epoch.Store(oldCurrentEpoch)

	c.state.current.c2s, c.state.staged.c2s =
		c.state.staged.c2s, c.state.current.c2s
	c.state.current.s2c, c.state.staged.s2c =
		c.state.staged.s2c, c.state.current.s2c

	c.clearStagedLocked()
	c.state.phase.Store(phaseRetiring)
	return true
}

func (c *StateMachine) clearStagedLocked() {
	staged := &c.state.staged
	staged.c2s = staged.c2s[:chacha20poly1305.KeySize]
	staged.s2c = staged.s2c[:chacha20poly1305.KeySize]
	securemem.ZeroBytes(staged.c2s)
	securemem.ZeroBytes(staged.s2c)
}
