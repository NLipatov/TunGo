package noise

import (
	"bytes"
	"fmt"
)

// StartRekeyV2 creates the first Noise IK message of a rehandshake.
func (h *IKHandshake) StartRekeyV2(prologue []byte) ([]byte, error) {
	if !h.Supports(CapabilityRekeyV2) {
		return nil, fmt.Errorf("noise: rekey v2 was not negotiated")
	}
	if err := h.validateClientConfig(); err != nil {
		return nil, err
	}
	if h.pendingRekey != nil {
		return nil, fmt.Errorf("noise: rekey already in progress")
	}
	hs, err := h.newInitiatorState(prologue)
	if err != nil {
		return nil, err
	}
	msg1, _, _, err := hs.WriteMessage(nil, nil)
	if err != nil {
		zeroizeLocalEphemeral(hs)
		return nil, fmt.Errorf("noise: write rekey msg1: %w", err)
	}
	h.pendingRekey = hs
	return msg1, nil
}

// FinishRekeyV2 consumes the responder's Noise IK message and returns the new
// canonical client-to-server and server-to-client traffic keys.
func (h *IKHandshake) FinishRekeyV2(msg2 []byte) ([]byte, []byte, error) {
	hs := h.pendingRekey
	if hs == nil {
		return nil, nil, fmt.Errorf("noise: no rekey in progress")
	}
	h.pendingRekey = nil
	defer zeroizeLocalEphemeral(hs)

	_, cs1, cs2, err := hs.ReadMessage(nil, msg2)
	if err != nil {
		return nil, nil, fmt.Errorf("noise: read rekey msg2: %w", err)
	}
	if cs1 == nil || cs2 == nil {
		return nil, nil, fmt.Errorf("noise: rekey handshake not complete after msg2")
	}
	if !bytes.Equal(hs.PeerStatic(), h.peerPubKey) {
		return nil, nil, fmt.Errorf("noise: server static key mismatch")
	}
	material := extractSessionMaterial(cs1, cs2, hs.ChannelBinding())
	return material.clientKey, material.serverKey, nil
}

// RespondRekeyV2 consumes the initiator's Noise IK message, authenticates the
// same client as the original session, and returns msg2 plus new traffic keys.
func (h *IKHandshake) RespondRekeyV2(prologue, msg1 []byte) ([]byte, []byte, []byte, error) {
	if !h.Supports(CapabilityRekeyV2) {
		return nil, nil, nil, fmt.Errorf("noise: rekey v2 was not negotiated")
	}
	if err := h.validateServerConfig(); err != nil {
		return nil, nil, nil, err
	}
	hs, err := h.newResponderState(prologue)
	if err != nil {
		return nil, nil, nil, err
	}
	defer zeroizeLocalEphemeral(hs)

	_, _, _, err = hs.ReadMessage(nil, msg1)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("noise: read rekey msg1: %w", err)
	}
	clientPubKey := hs.PeerStatic()
	if !bytes.Equal(clientPubKey, h.authenticatedClientPubKey) {
		return nil, nil, nil, ErrUnknownPeer
	}
	_, enabled, found := h.allowedPeers.Lookup(clientPubKey)
	if !found {
		return nil, nil, nil, ErrUnknownPeer
	}
	if !enabled {
		return nil, nil, nil, ErrPeerDisabled
	}

	msg2, cs1, cs2, err := hs.WriteMessage(nil, nil)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("noise: write rekey msg2: %w", err)
	}
	if cs1 == nil || cs2 == nil {
		return nil, nil, nil, fmt.Errorf("noise: rekey handshake not complete after msg2")
	}
	material := extractSessionMaterial(cs1, cs2, hs.ChannelBinding())
	return msg2, material.clientKey, material.serverKey, nil
}
