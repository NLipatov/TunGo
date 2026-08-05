package tcp_chacha20

import (
	"errors"

	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/logging"
	"tungo/infrastructure/network/service_packet"
	"tungo/infrastructure/settings"
)

type rekeyController interface {
	HandleRekeyInit(
		carrierEpoch uint16,
		crypto primitives.KeyDeriver,
		plaindata []byte,
	) (serverPub [service_packet.RekeyPublicKeyLen]byte, epoch uint16, ok bool, err error)
	ActivateSendEpoch(epoch uint16)
}

// controlPlaneHandler is a dataplane-adapter for inbound control-plane packets.
// It delegates protocol logic to tunnel/controlplane.
//
// Key difference from UDP: TCP activates the send epoch after sending
// ACK (stream protocol — explicit activation). UDP activates based on received
// packet epoch.
type controlPlaneHandler struct {
	crypto       primitives.KeyDeriver
	logger       logging.Logger
	ackBuf       [epochPrefixSize + service_packet.RekeyPacketLen + settings.TCPChacha20Overhead]byte
	exhaustedBuf [epochPrefixSize + 3 + settings.TCPChacha20Overhead]byte
	pongBuf      [epochPrefixSize + 3 + settings.TCPChacha20Overhead]byte
}

func newControlPlaneHandler(crypto primitives.KeyDeriver, logger logging.Logger) controlPlaneHandler {
	return controlPlaneHandler{
		crypto: crypto,
		logger: logger,
	}
}

func (h *controlPlaneHandler) Handle(
	carrierEpoch uint16,
	plaindata []byte,
	egress connection.Egress,
	controller rekeyController,
) (bool, error) {
	if spType, ok := service_packet.TryParseHeader(plaindata); ok {
		switch spType {
		case service_packet.RekeyInit:
			return true, h.handleRekeyInit(carrierEpoch, plaindata, egress, controller)
		default:
			return true, nil
		}
	}
	return false, nil
}

// handleRekeyInit processes a rekey init packet.
func (h *controlPlaneHandler) handleRekeyInit(
	carrierEpoch uint16,
	plaindata []byte,
	egress connection.Egress,
	controller rekeyController,
) error {
	if controller == nil {
		return nil
	}
	// 1. Derive keys and stage through the controller (StageEpoch adds a new epoch session
	//    but does NOT change the send epoch — outbound frames still use old key).
	serverPub, epoch, ok, err := controller.HandleRekeyInit(carrierEpoch, h.crypto, plaindata)
	if err != nil {
		h.logger.Warn("rekey init failed", "err", err)
		if errors.Is(err, chacha20.ErrEpochExhausted) {
			// Send EpochExhausted to notify client to reconnect.
			// Session stays alive - client will reconnect, then this session closes.
			h.sendEpochExhausted(egress)
		}
		return nil
	}
	if !ok {
		return nil
	}

	// 2. Build and send ACK. Because sendEpoch is still the old epoch, the ACK
	//    is encrypted with the old key — the client can always decrypt it.
	// Reserve first 2 bytes for epoch prefix (written by Crypto.Encrypt).
	ackPayload := h.ackBuf[epochPrefixSize : epochPrefixSize+service_packet.RekeyPacketLen]
	copy(ackPayload[3:], serverPub[:])
	sp, err := service_packet.EncodeV1Header(service_packet.RekeyAck, ackPayload)
	if err != nil {
		h.logger.Error("rekey init encode ack failed", "err", err)
		return err
	}
	// Prepend epoch prefix reservation to the service packet.
	spWithPrefix := h.ackBuf[:epochPrefixSize+len(sp)]
	if err := egress.SendControl(spWithPrefix); err != nil {
		h.logger.Error("rekey init send ack failed", "err", err)
		return err
	}

	// 3. Now switch send to the new epoch — all subsequent frames use new key.
	controller.ActivateSendEpoch(epoch)
	return nil
}

func (h *controlPlaneHandler) HandlePing(egress connection.Egress) {
	payload := h.pongBuf[epochPrefixSize : epochPrefixSize+3]
	if _, err := service_packet.EncodeV1Header(service_packet.Pong, payload); err != nil {
		h.logger.Error("pong encode failed", "err", err)
		return
	}
	spWithPrefix := h.pongBuf[:epochPrefixSize+3]
	if err := egress.SendControl(spWithPrefix); err != nil {
		h.logger.Error("pong send failed", "err", err)
	}
}

func (h *controlPlaneHandler) sendEpochExhausted(egress connection.Egress) {
	payload := h.exhaustedBuf[epochPrefixSize : epochPrefixSize+3]
	sp, err := service_packet.EncodeV1Header(service_packet.EpochExhausted, payload)
	if err != nil {
		return
	}
	spWithPrefix := h.exhaustedBuf[:epochPrefixSize+len(sp)]
	_ = egress.SendControl(spWithPrefix)
}
