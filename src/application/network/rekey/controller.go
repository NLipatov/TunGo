package rekey

import (
	"fmt"
	"sync"
)

// Rekeyer is the minimal interface the controller needs from the crypto layer.
// It returns the new epoch so callers can keep control-plane state consistent.
type Rekeyer interface {
	Rekey(sendKey, recvKey []byte) (uint16, error)
}

// Controller holds control-plane rekey state; crypto remains immutable and handshake-agnostic.
// It is intentionally not in the cryptography package to separate concerns.
type Controller struct {
	mu             sync.Mutex
	Crypto         Rekeyer
	IsServer       bool
	CurrentC2S     []byte
	CurrentS2C     []byte
	PendingPriv    *[32]byte
	LastRekeyEpoch uint16
}

func NewController(core Rekeyer, c2s, s2c []byte, isServer bool) *Controller {
	return &Controller{
		Crypto:     core,
		IsServer:   isServer,
		CurrentC2S: append([]byte(nil), c2s...),
		CurrentS2C: append([]byte(nil), s2c...),
	}
}

func (c *Controller) SetPendingRekeyPrivateKey(priv [32]byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.PendingPriv = &priv
}

func (c *Controller) PendingRekeyPrivateKey() ([32]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.PendingPriv == nil {
		return [32]byte{}, false
	}
	return *c.PendingPriv, true
}

func (c *Controller) ClearPendingRekeyPrivateKey() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.PendingPriv = nil
}

func (c *Controller) CurrentClientToServerKey() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.CurrentC2S...)
}

func (c *Controller) CurrentServerToClientKey() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.CurrentS2C...)
}

// ApplyKeys updates the stored raw keys after a successful rekey operation.
// sendKey/recvKey follow the same orientation as the crypto: sendKey is for
// outbound traffic of this peer. epoch must be strictly increasing.
func (c *Controller) ApplyKeys(sendKey, recvKey []byte, epoch uint16) error {
	c.mu.Lock()
	defer c.mu.Unlock()

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
	c.LastRekeyEpoch = epoch
	return nil
}
