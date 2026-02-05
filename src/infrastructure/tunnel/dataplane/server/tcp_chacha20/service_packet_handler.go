package tcp_chacha20

import (
	"errors"

	"tungo/application/logging"
	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/network/service_packet"
	"tungo/infrastructure/settings"
	"tungo/infrastructure/tunnel/controlplane"
)

// controlPlaneHandler is a dataplane-adapter for inbound control-plane packets.
// It delegates protocol logic to infrastructure/routing/controlplane.
//
// Key difference from UDP: TCP calls fsm.ActivateSendEpoch(epoch) after sending
// ACK (stream protocol — explicit activation). UDP activates based on received
// packet epoch.
type controlPlaneHandler struct {
	crypto       primitives.KeyDeriver
	logger       logging.Logger
	ackBuf       [service_packet.RekeyPacketLen + settings.TCPChacha20Overhead]byte
	exhaustedBuf [3 + settings.TCPChacha20Overhead]byte
}

func newControlPlaneHandler(crypto primitives.KeyDeriver, logger logging.Logger) controlPlaneHandler {
	return controlPlaneHandler{
		crypto: crypto,
		logger: logger,
	}
}

func (h *controlPlaneHandler) Handle(
	plaindata []byte,
	egress connection.Egress,
	fsm rekey.FSM,
) bool {
	if spType, ok := service_packet.TryParseHeader(plaindata); ok {
		switch spType {
		case service_packet.RekeyInit:
			h.handleRekeyInit(plaindata, egress, fsm)
			return true
		default:
			return true
		}
	}
	return false
}

// handleRekeyInit processes a rekey init packet.
func (h *controlPlaneHandler) handleRekeyInit(
	plaindata []byte,
	egress connection.Egress,
	fsm rekey.FSM,
) {
	// 1. Derive keys and install into the FSM (Rekey adds a new epoch session
	//    but does NOT change the send epoch — outbound frames still use old key).
	serverPub, epoch, ok, err := controlplane.ServerHandleRekeyInit(h.crypto, fsm, plaindata)
	if err != nil {
		h.logger.Printf("rekey init: %v", err)
		if errors.Is(err, rekey.ErrEpochExhausted) {
			// Send EpochExhausted to notify client to reconnect.
			// Session stays alive - client will reconnect, then this session closes.
			h.sendEpochExhausted(egress)
		}
		return
	}
	if !ok {
		return
	}

	// 2. Build and send ACK. Because sendEpoch is still the old epoch, the ACK
	//    is encrypted with the old key — the client can always decrypt it.
	ackPayload := h.ackBuf[:service_packet.RekeyPacketLen]
	copy(ackPayload[3:], serverPub)
	sp, err := service_packet.EncodeV1Header(service_packet.RekeyAck, ackPayload)
	if err != nil {
		h.logger.Printf("rekey init: encode ack failed: %v", err)
		return
	}
	if err := egress.SendControl(sp); err != nil {
		h.logger.Printf("rekey init: send ack failed: %v", err)
		return
	}

	// 3. Now switch send to the new epoch — all subsequent frames use new key.
	fsm.ActivateSendEpoch(epoch)
}

func (h *controlPlaneHandler) sendEpochExhausted(egress connection.Egress) {
	payload := h.exhaustedBuf[:3]
	sp, err := service_packet.EncodeV1Header(service_packet.EpochExhausted, payload)
	if err != nil {
		return
	}
	_ = egress.SendControl(sp)
}
