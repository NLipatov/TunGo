package rekey

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
	ObservePeerEpoch(epoch uint16)
}

type clientRekeyV2Handshake interface {
	StartRekeyV2(prologue []byte) ([]byte, error)
	FinishRekeyV2(msg2 []byte) (c2s, s2c []byte, err error)
}

// ClientRekeyCoordinator owns the client-side RekeyInit/RekeyAck exchange.
// It is shared by the TUN and transport handlers of one connection.
type ClientRekeyCoordinator struct {
	mu sync.Mutex

	crypto     primitives.KeyDeriver
	controller clientRekeyController
	v2         clientRekeyV2Handshake
	interval   time.Duration
	rotateAt   time.Time

	pendingPrivateKey    [32]byte
	pendingV2Init        []byte
	pendingCarrierEpoch  uint16
	hasPendingPrivateKey bool
}

func NewClientRekeyCoordinator(
	crypto primitives.KeyDeriver,
	controller clientRekeyController,
	v2 clientRekeyV2Handshake,
	interval time.Duration,
	now time.Time,
) *ClientRekeyCoordinator {
	return &ClientRekeyCoordinator{
		crypto:     crypto,
		controller: controller,
		v2:         v2,
		interval:   interval,
		rotateAt:   now.Add(interval),
	}
}

// MaybeBuildRekeyInit returns the negotiated rekey-init service packet in dst.
// ok=false means "do nothing".
func (c *ClientRekeyCoordinator) MaybeBuildRekeyInit(
	now time.Time,
	dst []byte,
) (payload []byte, ok bool, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.controller == nil || (c.v2 == nil && c.crypto == nil) {
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
	if c.v2 != nil {
		return c.buildRekeyInitV2Locked(dst)
	}
	if len(dst) < v1PacketLen {
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
	if len(publicKey) != v1PublicKeyLen {
		return nil, false, nil
	}

	payload = dst[:v1PacketLen]
	copy(payload[serviceHeaderLen:], publicKey)
	if err := service_packet.Encode(service_packet.RekeyInit, payload); err != nil {
		return nil, false, err
	}
	return payload, true, nil
}

func (c *ClientRekeyCoordinator) buildRekeyInitV2Locked(dst []byte) ([]byte, bool, error) {
	if c.pendingV2Init == nil {
		prologue := rekeyV2Prologue(c.controller)
		msg1, err := c.v2.StartRekeyV2(prologue[:])
		if err != nil {
			return nil, false, err
		}
		c.pendingV2Init = msg1
		c.pendingCarrierEpoch = c.controller.SendEpoch()
	} else if c.controller.SendEpoch() != c.pendingCarrierEpoch {
		return nil, false, fmt.Errorf("pending rekey carrier epoch changed")
	}

	packetLen := serviceHeaderLen + len(c.pendingV2Init)
	if len(dst) < packetLen {
		return nil, false, nil
	}
	payload := dst[:packetLen]
	copy(payload[serviceHeaderLen:], c.pendingV2Init)
	if err := service_packet.Encode(service_packet.RekeyInitV2, payload); err != nil {
		return nil, false, err
	}
	return payload, true, nil
}

// HandleRekeyAck completes the client-side exchange and promotes the new send epoch.
func (c *ClientRekeyCoordinator) HandleRekeyAck(
	carrierEpoch uint16,
	plaindata []byte,
) (ok bool, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.controller == nil || (c.v2 == nil && c.crypto == nil) {
		return false, nil
	}
	if c.v2 != nil {
		return c.handleRekeyAckV2Locked(carrierEpoch, plaindata)
	}
	kind, parsed := service_packet.Parse(plaindata)
	if !parsed || kind != service_packet.RekeyAck || len(plaindata) != v1PacketLen || !c.hasPendingPrivateKey {
		return false, nil
	}
	if carrierEpoch != c.pendingCarrierEpoch {
		return false, nil
	}

	serverPub := plaindata[serviceHeaderLen:v1PacketLen]
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

func (c *ClientRekeyCoordinator) handleRekeyAckV2Locked(
	carrierEpoch uint16,
	plaindata []byte,
) (bool, error) {
	kind, ok := service_packet.Parse(plaindata)
	if !ok || kind != service_packet.RekeyAckV2 || len(plaindata) <= serviceHeaderLen || c.pendingV2Init == nil {
		return false, nil
	}
	if carrierEpoch != c.pendingCarrierEpoch {
		return false, nil
	}
	c2s, s2c, err := c.v2.FinishRekeyV2(plaindata[serviceHeaderLen:])
	c.pendingV2Init = nil
	c.pendingCarrierEpoch = 0
	if err != nil {
		return false, err
	}
	defer mem.ZeroBytes(c2s)
	defer mem.ZeroBytes(s2c)

	epoch, err := c.controller.StartRekey(c2s, s2c)
	if err != nil {
		return false, err
	}
	c.controller.ActivateSendEpoch(epoch)
	return true, nil
}

func (c *ClientRekeyCoordinator) ObservePeerEpoch(epoch uint16) {
	if c != nil && c.controller != nil {
		c.controller.ObservePeerEpoch(epoch)
	}
}

func (c *ClientRekeyCoordinator) ActivateSendEpoch(epoch uint16) {
	if c != nil && c.controller != nil {
		c.controller.ActivateSendEpoch(epoch)
	}
}

func (c *ClientRekeyCoordinator) clearPendingPrivateKeyLocked() {
	mem.ZeroBytes(c.pendingPrivateKey[:])
	c.pendingCarrierEpoch = 0
	c.hasPendingPrivateKey = false
}
