package rekey

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"sync/atomic"

	"tungo/infrastructure/cryptography/mem"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/network/service_packet"

	"golang.org/x/crypto/curve25519"
)

type serverRekeyTransaction struct {
	requestHash   [sha256.Size]byte
	response      []byte
	carrierEpoch  uint16
	epoch         uint16
	sendActivated atomic.Bool
	peerObserved  atomic.Bool
}

type epochController interface {
	ReadyForRekey() bool
	StartRekey(c2s, s2c []byte) (uint16, error)
	ActivateSendEpoch(epoch uint16)
	ObservePeerEpoch(epoch uint16)
	CurrentKeys() (clientToServer, serverToClient []byte)
}

type serverRekeyV2Handshake interface {
	RespondRekeyV2(prologue, msg1 []byte) (msg2, c2s, s2c []byte, err error)
}

func (t *serverRekeyTransaction) complete() bool {
	return t.sendActivated.Load() && t.peerObserved.Load()
}

// ServerRekeyCoordinator makes rekey-init handling idempotent for one session.
// Repeated requests return the same response and staged epoch; completed
// replays are ignored.
type ServerRekeyCoordinator struct {
	mu         sync.Mutex
	controller epochController
	v2         serverRekeyV2Handshake
	current    atomic.Pointer[serverRekeyTransaction]
}

func NewServerRekeyCoordinator(
	controller epochController,
	v2 serverRekeyV2Handshake,
) *ServerRekeyCoordinator {
	return &ServerRekeyCoordinator{controller: controller, v2: v2}
}

var (
	deriveLabelC2S = []byte("tungo-rekey-c2s")
	deriveLabelS2C = []byte("tungo-rekey-s2c")
)

// Handle consumes either rekey-init packet and returns an encoded ack packet.
// It performs no IO. ok=false means there is no response to send.
func (c *ServerRekeyCoordinator) Handle(
	carrierEpoch uint16,
	crypto primitives.KeyDeriver,
	plaindata []byte,
) (response []byte, epoch uint16, ok bool, err error) {
	if c == nil || c.controller == nil {
		return nil, 0, false, nil
	}
	kind, ok := service_packet.Parse(plaindata)
	if !ok || (kind != service_packet.RekeyInit && kind != service_packet.RekeyInitV2) {
		return nil, 0, false, nil
	}
	if kind == service_packet.RekeyInit && (c.v2 != nil || crypto == nil || len(plaindata) != v1PacketLen) {
		return nil, 0, false, nil
	}
	if kind == service_packet.RekeyInitV2 && (c.v2 == nil || len(plaindata) <= serviceHeaderLen) {
		return nil, 0, false, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	requestHash := sha256.Sum256(plaindata)
	current := c.current.Load()
	if current != nil && current.requestHash == requestHash {
		if current.complete() || current.carrierEpoch != carrierEpoch {
			return nil, 0, false, nil
		}
		return append([]byte(nil), current.response...), current.epoch, true, nil
	}
	if current != nil {
		if !current.complete() || carrierEpoch != current.epoch {
			return nil, 0, false, nil
		}
	}
	if !c.controller.ReadyForRekey() {
		return nil, 0, false, nil
	}

	var body, c2s, s2c []byte
	var responseType service_packet.HeaderType
	switch kind {
	case service_packet.RekeyInit:
		body, c2s, s2c, err = c.rekeyV1(crypto, plaindata)
		responseType = service_packet.RekeyAck
	case service_packet.RekeyInitV2:
		prologue := rekeyV2Prologue(c.controller)
		body, c2s, s2c, err = c.v2.RespondRekeyV2(prologue[:], plaindata[serviceHeaderLen:])
		responseType = service_packet.RekeyAckV2
	}
	if err != nil {
		return nil, 0, false, err
	}
	defer mem.ZeroBytes(c2s)
	defer mem.ZeroBytes(s2c)

	epoch, err = c.controller.StartRekey(c2s, s2c)
	if err != nil {
		return nil, 0, false, err
	}
	response = make([]byte, serviceHeaderLen+len(body))
	if err := service_packet.Encode(responseType, response); err != nil {
		return nil, 0, false, err
	}
	copy(response[serviceHeaderLen:], body)

	c.current.Store(&serverRekeyTransaction{
		requestHash:  requestHash,
		response:     append([]byte(nil), response...),
		carrierEpoch: carrierEpoch,
		epoch:        epoch,
	})
	return response, epoch, true, nil
}

func (c *ServerRekeyCoordinator) rekeyV1(
	crypto primitives.KeyDeriver,
	plaindata []byte,
) (serverPub, c2s, s2c []byte, err error) {
	generatedPub, serverPriv, err := crypto.GenerateX25519KeyPair()
	if err != nil {
		return nil, nil, nil, err
	}
	defer mem.ZeroBytes(serverPriv[:])
	shared, err := curve25519.X25519(serverPriv[:], plaindata[serviceHeaderLen:v1PacketLen])
	if err != nil {
		return nil, nil, nil, err
	}
	defer mem.ZeroBytes(shared)

	currentC2S, currentS2C := c.controller.CurrentKeys()
	defer mem.ZeroBytes(currentC2S)
	defer mem.ZeroBytes(currentS2C)
	c2s, err = crypto.DeriveKey(shared, currentC2S, deriveLabelC2S)
	if err != nil {
		return nil, nil, nil, err
	}
	s2c, err = crypto.DeriveKey(shared, currentS2C, deriveLabelS2C)
	if err != nil {
		mem.ZeroBytes(c2s)
		return nil, nil, nil, err
	}
	if len(generatedPub) != v1PublicKeyLen {
		mem.ZeroBytes(c2s)
		mem.ZeroBytes(s2c)
		return nil, nil, nil, fmt.Errorf("unexpected server public key length: %d", len(generatedPub))
	}
	return generatedPub, c2s, s2c, nil
}

func rekeyV2Prologue(controller epochController) [sha256.Size]byte {
	c2s, s2c := controller.CurrentKeys()
	defer mem.ZeroBytes(c2s)
	defer mem.ZeroBytes(s2c)

	hash := sha256.New()
	_, _ = hash.Write([]byte("tungo-rekey-v2"))
	_, _ = hash.Write(c2s)
	_, _ = hash.Write(s2c)
	var prologue [sha256.Size]byte
	copy(prologue[:], hash.Sum(nil))
	return prologue
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
