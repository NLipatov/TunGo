package controlplane

import (
	"fmt"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/network/service_packet"

	"golang.org/x/crypto/curve25519"
)

var (
	deriveLabelC2S = []byte("tungo-rekey-c2s")
	deriveLabelS2C = []byte("tungo-rekey-s2c")
)

// ServerHandleRekeyInit parses a RekeyInit packet and installs new keys in the FSM.
//
// It does not do any IO; caller is responsible for sending RekeyAck with the returned serverPub.
// Returns ok=false when packet should be ignored/dropped (non-stable FSM, short packet, etc).
func ServerHandleRekeyInit(
	crypto primitives.KeyDeriver,
	fsm rekey.FSM,
	plaindata []byte,
) (serverPub []byte, epoch uint16, ok bool, err error) {
	if fsm == nil || crypto == nil {
		return nil, 0, false, nil
	}
	if fsm.State() != rekey.StateStable {
		return nil, 0, false, nil
	}
	if len(plaindata) < service_packet.RekeyPacketLen {
		return nil, 0, false, nil
	}

	var clientPub [service_packet.RekeyPublicKeyLen]byte
	copy(clientPub[:], plaindata[3:service_packet.RekeyPacketLen])

	serverPub, serverPriv, err := crypto.GenerateX25519KeyPair()
	if err != nil {
		return nil, 0, false, err
	}
	shared, err := curve25519.X25519(serverPriv[:], clientPub[:])
	if err != nil {
		return nil, 0, false, err
	}

	currentC2S := fsm.CurrentClientToServerKey()
	currentS2C := fsm.CurrentServerToClientKey()
	newC2S, err := crypto.DeriveKey(shared, currentC2S, deriveLabelC2S)
	if err != nil {
		return nil, 0, false, err
	}
	newS2C, err := crypto.DeriveKey(shared, currentS2C, deriveLabelS2C)
	if err != nil {
		return nil, 0, false, err
	}

	sendKey := newC2S
	recvKey := newS2C
	if fsm.IsServer() {
		sendKey, recvKey = newS2C, newC2S // server sends S2C, receives C2S
	}

	if len(serverPub) != service_packet.RekeyPublicKeyLen {
		return nil, 0, false, fmt.Errorf("unexpected server public key length: %d", len(serverPub))
	}

	epoch, err = fsm.StartRekey(sendKey, recvKey)
	if err != nil {
		return nil, 0, false, err
	}

	return serverPub, epoch, true, nil
}

// ClientHandleRekeyAck parses a RekeyAck packet and installs new keys in the FSM.
//
// It switches the local send side immediately (ActivateSendEpoch) and clears the pending private key.
// Returns ok=false when packet should be ignored/dropped (no pending key, short packet, etc).
func ClientHandleRekeyAck(
	crypto primitives.KeyDeriver,
	fsm *rekey.StateMachine,
	plaindata []byte,
) (ok bool, err error) {
	if fsm == nil || crypto == nil {
		return false, nil
	}
	if len(plaindata) < service_packet.RekeyPacketLen {
		return false, nil
	}

	priv, ok := fsm.PendingRekeyPrivateKey()
	if !ok {
		return false, nil
	}

	serverPub := plaindata[3 : 3+service_packet.RekeyPublicKeyLen]
	shared, err := curve25519.X25519(priv[:], serverPub)
	if err != nil {
		return false, err
	}

	currentC2S := fsm.CurrentClientToServerKey()
	currentS2C := fsm.CurrentServerToClientKey()
	newC2S, err := crypto.DeriveKey(shared, currentC2S, deriveLabelC2S)
	if err != nil {
		return false, err
	}
	newS2C, err := crypto.DeriveKey(shared, currentS2C, deriveLabelS2C)
	if err != nil {
		return false, err
	}

	epoch, err := fsm.StartRekey(newC2S, newS2C)
	if err != nil {
		return false, err
	}

	// Initiator proactively switches send to drive peer confirmation.
	fsm.ActivateSendEpoch(epoch)
	fsm.ClearPendingRekeyPrivateKey()
	return true, nil
}
