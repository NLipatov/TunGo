package udp_chacha20

import (
	"errors"
	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/network/service_packet"
	"tungo/infrastructure/tunnel/controlplane"

	"golang.org/x/crypto/chacha20poly1305"
)

// controlPlaneHandler is a dataplane-adapter for inbound control-plane packets.
// It delegates protocol logic to infrastructure/routing/controlplane.
type controlPlaneHandler struct {
	crypto       primitives.KeyDeriver
	ackBuf       [chacha20poly1305.NonceSize + service_packet.RekeyPacketLen + chacha20poly1305.Overhead]byte
	pongBuf      [chacha20poly1305.NonceSize + 3 + chacha20poly1305.Overhead]byte
	exhaustedBuf [chacha20poly1305.NonceSize + 3 + chacha20poly1305.Overhead]byte
}

func newServicePacketHandler(
	crypto primitives.KeyDeriver,
) controlPlaneHandler {
	return controlPlaneHandler{
		crypto: crypto,
	}
}

func (r *controlPlaneHandler) Handle(
	plaindata []byte,
	egress connection.Egress,
	fsm rekey.FSM,
) (bool, error) {
	if spType, ok := service_packet.TryParseHeader(plaindata); ok {
		switch spType {
		case service_packet.RekeyInit:
			return true, r.handleRekeyInit(plaindata, egress, fsm)
		case service_packet.Ping:
			return true, r.handlePing(egress)
		default:
			return true, nil
		}
	}
	return false, nil
}

func (r *controlPlaneHandler) handlePing(egress connection.Egress) error {
	buf := r.pongBuf[:chacha20poly1305.NonceSize+3]
	payload := buf[chacha20poly1305.NonceSize:]
	if _, err := service_packet.EncodeV1Header(service_packet.Pong, payload); err != nil {
		return nil
	}
	_ = egress.SendControl(buf)
	return nil
}

func (r *controlPlaneHandler) handleRekeyInit(
	plaindata []byte,
	egress connection.Egress,
	fsm rekey.FSM,
) error {
	serverPub, _, ok, err := controlplane.ServerHandleRekeyInit(r.crypto, fsm, plaindata)
	if err != nil {
		if errors.Is(err, rekey.ErrEpochExhausted) {
			// Send encrypted EpochExhausted to notify client to reconnect.
			r.sendEpochExhausted(egress)
			return err
		}
		return nil
	}
	if !ok {
		return nil
	}
	// Only send ACK after successful rekey installation.
	ackBuf := r.ackBuf[:chacha20poly1305.NonceSize+service_packet.RekeyPacketLen]
	payload := ackBuf[chacha20poly1305.NonceSize:]
	copy(payload[3:], serverPub)
	if _, err = service_packet.EncodeV1Header(service_packet.RekeyAck, payload); err != nil {
		return nil
	}
	if err := egress.SendControl(ackBuf); err != nil {
		return nil
	}
	return nil
}

func (r *controlPlaneHandler) sendEpochExhausted(egress connection.Egress) {
	buf := r.exhaustedBuf[:chacha20poly1305.NonceSize+3]
	payload := buf[chacha20poly1305.NonceSize:]
	if _, err := service_packet.EncodeV1Header(service_packet.EpochExhausted, payload); err != nil {
		return
	}
	_ = egress.SendControl(buf)
}
