package controlplane

import (
	"fmt"
	"sync"
	"time"

	"tungo/infrastructure/cryptography/mem"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/network/service_packet"

	"golang.org/x/crypto/curve25519"
)

type clientRekeyController interface {
	ReadyForRekey() bool
	SendEpoch() uint16
	CurrentKeys() (clientToServer, serverToClient []byte)
	StartRekey(c2s, s2c []byte) (uint16, error)
	ActivateSendEpoch(epoch uint16)
}

// ClientRekeyCoordinator owns the client-side RekeyInit/RekeyAck exchange.
// It is shared by the TUN and transport handlers of one connection.
type ClientRekeyCoordinator struct {
	mu sync.Mutex

	crypto     primitives.KeyDeriver
	controller clientRekeyController
	interval   time.Duration
	rotateAt   time.Time

	pendingPrivateKey    [32]byte
	pendingCarrierEpoch  uint16
	hasPendingPrivateKey bool
}

func NewClientRekeyCoordinator(
	crypto primitives.KeyDeriver,
	controller clientRekeyController,
	interval time.Duration,
	now time.Time,
) *ClientRekeyCoordinator {
	return &ClientRekeyCoordinator{
		crypto:     crypto,
		controller: controller,
		interval:   interval,
		rotateAt:   now.Add(interval),
	}
}

// MaybeBuildRekeyInit returns a v1 service_packet.RekeyInit plaintext payload in dst.
// ok=false means "do nothing".
func (c *ClientRekeyCoordinator) MaybeBuildRekeyInit(
	now time.Time,
	dst []byte,
) (payload []byte, ok bool, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.crypto == nil || c.controller == nil {
		return nil, false, nil
	}
	if now.Before(c.rotateAt) {
		return nil, false, nil
	}
	// We are due; schedule next attempt regardless of outcome.
	c.rotateAt = now.Add(c.interval)
	if !c.controller.ReadyForRekey() {
		return nil, false, nil
	}
	if len(dst) < service_packet.RekeyPacketLen {
		return nil, false, nil
	}

	var publicKey []byte
	if c.hasPendingPrivateKey {
		if c.controller.SendEpoch() != c.pendingCarrierEpoch {
			return nil, false, fmt.Errorf("pending rekey carrier epoch changed")
		}
		// Reuse the in-flight key to avoid mismatched ACKs.
		publicKey, err = curve25519.X25519(c.pendingPrivateKey[:], curve25519.Basepoint)
	} else {
		var privateKey [32]byte
		publicKey, privateKey, err = c.crypto.GenerateX25519KeyPair()
		if err == nil {
			c.pendingPrivateKey = privateKey
			c.pendingCarrierEpoch = c.controller.SendEpoch()
			c.hasPendingPrivateKey = true
		}
		mem.ZeroBytes(privateKey[:])
	}
	if err != nil {
		return nil, false, err
	}
	if len(publicKey) != service_packet.RekeyPublicKeyLen {
		return nil, false, nil
	}

	copy(dst[3:], publicKey)
	servicePayload, err := service_packet.EncodeV1Header(service_packet.RekeyInit, dst)
	if err != nil {
		return nil, false, err
	}
	return servicePayload, true, nil
}

// HandleRekeyAck completes the client-side exchange and promotes the new send epoch.
func (c *ClientRekeyCoordinator) HandleRekeyAck(
	carrierEpoch uint16,
	plaindata []byte,
) (ok bool, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.crypto == nil || c.controller == nil {
		return false, nil
	}
	if len(plaindata) < service_packet.RekeyPacketLen || !c.hasPendingPrivateKey {
		return false, nil
	}
	if carrierEpoch != c.pendingCarrierEpoch {
		return false, nil
	}

	serverPub := plaindata[3 : 3+service_packet.RekeyPublicKeyLen]
	shared, err := curve25519.X25519(c.pendingPrivateKey[:], serverPub)
	if err != nil {
		return false, err
	}
	defer mem.ZeroBytes(shared)

	currentC2S, currentS2C := c.controller.CurrentKeys()
	defer mem.ZeroBytes(currentC2S)
	defer mem.ZeroBytes(currentS2C)
	newC2S, err := c.crypto.DeriveKey(shared, currentC2S, deriveLabelC2S)
	if err != nil {
		return false, err
	}
	defer mem.ZeroBytes(newC2S)
	newS2C, err := c.crypto.DeriveKey(shared, currentS2C, deriveLabelS2C)
	if err != nil {
		return false, err
	}
	defer mem.ZeroBytes(newS2C)

	epoch, err := c.controller.StartRekey(newC2S, newS2C)
	if err != nil {
		return false, err
	}

	// Initiator proactively switches send to drive peer confirmation.
	c.controller.ActivateSendEpoch(epoch)
	c.clearPendingPrivateKeyLocked()
	return true, nil
}

func (c *ClientRekeyCoordinator) clearPendingPrivateKeyLocked() {
	mem.ZeroBytes(c.pendingPrivateKey[:])
	c.pendingCarrierEpoch = 0
	c.hasPendingPrivateKey = false
}
