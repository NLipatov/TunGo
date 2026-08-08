package rekey

import (
	"fmt"
	"sync"
	"sync/atomic"

	"tungo/infrastructure/cryptography/mem"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/network/service_packet"

	"golang.org/x/crypto/curve25519"
)

type serverRekeyTransaction struct {
	clientPub     [service_packet.RekeyPublicKeyLen]byte
	serverPub     [service_packet.RekeyPublicKeyLen]byte
	carrierEpoch  uint16
	epoch         uint16
	sendActivated atomic.Bool
	peerObserved  atomic.Bool
}

type epochController interface {
	ReadyForRekey() bool
	SendEpoch() uint16
	StartRekey(c2s, s2c []byte) (uint16, error)
	ActivateSendEpoch(epoch uint16)
	ObservePeerEpoch(epoch uint16)
	CurrentKeys() (clientToServer, serverToClient []byte)
}

func (t *serverRekeyTransaction) complete() bool {
	return t.sendActivated.Load() && t.peerObserved.Load()
}

// ServerRekeyCoordinator makes RekeyInit handling idempotent for one server
// session. While a transition is in flight, a repeated client public key maps
// to the same staged epoch and server public key; completed replays are ignored.
type ServerRekeyCoordinator struct {
	mu         sync.Mutex
	controller epochController
	current    atomic.Pointer[serverRekeyTransaction]
}

func NewServerRekeyCoordinator(controller epochController) *ServerRekeyCoordinator {
	return &ServerRekeyCoordinator{controller: controller}
}

var (
	deriveLabelC2S = []byte("tungo-rekey-c2s")
	deriveLabelS2C = []byte("tungo-rekey-s2c")
)

// HandleRekeyInit parses a RekeyInit packet and stages its keys. It does no IO;
// the caller sends RekeyAck with the returned server public key.
func (c *ServerRekeyCoordinator) HandleRekeyInit(
	carrierEpoch uint16,
	crypto primitives.KeyDeriver,
	plaindata []byte,
) (serverPub [service_packet.RekeyPublicKeyLen]byte, epoch uint16, ok bool, err error) {
	if c == nil || c.controller == nil || crypto == nil {
		return serverPub, 0, false, nil
	}
	if len(plaindata) < service_packet.RekeyPacketLen {
		return serverPub, 0, false, nil
	}

	var clientPub [service_packet.RekeyPublicKeyLen]byte
	copy(clientPub[:], plaindata[3:service_packet.RekeyPacketLen])

	c.mu.Lock()
	defer c.mu.Unlock()

	current := c.current.Load()
	if current != nil && current.clientPub == clientPub {
		if current.complete() {
			return serverPub, 0, false, nil
		}
		if current.carrierEpoch != carrierEpoch {
			return serverPub, 0, false, nil
		}
		return current.serverPub, current.epoch, true, nil
	}
	if current != nil {
		if !current.complete() || carrierEpoch != current.epoch {
			return serverPub, 0, false, nil
		}
	}
	if !c.controller.ReadyForRekey() {
		return serverPub, 0, false, nil
	}

	generatedPub, serverPriv, err := crypto.GenerateX25519KeyPair()
	if err != nil {
		return serverPub, 0, false, err
	}
	defer mem.ZeroBytes(serverPriv[:])
	shared, err := curve25519.X25519(serverPriv[:], clientPub[:])
	if err != nil {
		return serverPub, 0, false, err
	}
	defer mem.ZeroBytes(shared)

	currentC2S, currentS2C := c.controller.CurrentKeys()
	defer mem.ZeroBytes(currentC2S)
	defer mem.ZeroBytes(currentS2C)
	newC2S, err := crypto.DeriveKey(shared, currentC2S, deriveLabelC2S)
	if err != nil {
		return serverPub, 0, false, err
	}
	defer mem.ZeroBytes(newC2S)
	newS2C, err := crypto.DeriveKey(shared, currentS2C, deriveLabelS2C)
	if err != nil {
		return serverPub, 0, false, err
	}
	defer mem.ZeroBytes(newS2C)

	if len(generatedPub) != service_packet.RekeyPublicKeyLen {
		return serverPub, 0, false, fmt.Errorf("unexpected server public key length: %d", len(generatedPub))
	}
	copy(serverPub[:], generatedPub)

	epoch, err = c.controller.StartRekey(newC2S, newS2C)
	if err != nil {
		return serverPub, 0, false, err
	}

	c.current.Store(&serverRekeyTransaction{
		clientPub:    clientPub,
		serverPub:    serverPub,
		carrierEpoch: carrierEpoch,
		epoch:        epoch,
	})
	return serverPub, epoch, true, nil
}

func (c *ServerRekeyCoordinator) ReadyForRekey() bool {
	if c == nil || c.controller == nil {
		return false
	}
	current := c.current.Load()
	return (current == nil || current.complete()) && c.controller.ReadyForRekey()
}

func (c *ServerRekeyCoordinator) SendEpoch() uint16 {
	if c == nil || c.controller == nil {
		return 0
	}
	return c.controller.SendEpoch()
}

func (c *ServerRekeyCoordinator) StartRekey(c2s, s2c []byte) (uint16, error) {
	return c.controller.StartRekey(c2s, s2c)
}

func (c *ServerRekeyCoordinator) ActivateSendEpoch(epoch uint16) {
	if c == nil || c.controller == nil {
		return
	}
	c.controller.ActivateSendEpoch(epoch)
	if current := c.current.Load(); current != nil && current.epoch == epoch {
		current.sendActivated.Store(true)
	}
}

func (c *ServerRekeyCoordinator) ObservePeerEpoch(epoch uint16) {
	if c == nil || c.controller == nil {
		return
	}
	c.controller.ObservePeerEpoch(epoch)
	if current := c.current.Load(); current != nil && current.epoch == epoch {
		current.peerObserved.Store(true)
	}
}

func (c *ServerRekeyCoordinator) CurrentKeys() ([]byte, []byte) {
	if c == nil || c.controller == nil {
		return nil, nil
	}
	return c.controller.CurrentKeys()
}
