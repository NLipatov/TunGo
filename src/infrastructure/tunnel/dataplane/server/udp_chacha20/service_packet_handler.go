package udp_chacha20

import (
	"errors"
	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/udp"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/network/service_packet"

	"golang.org/x/crypto/chacha20poly1305"
)

type rekeyController interface {
	HandleRekeyInit(
		carrierEpoch uint16,
		crypto primitives.KeyDeriver,
		plaindata []byte,
	) (serverPub [service_packet.RekeyPublicKeyLen]byte, epoch uint16, ok bool, err error)
}

// controlPlaneHandler is a dataplane-adapter for inbound control-plane packets.
// It delegates protocol logic to tunnel/controlplane.
type controlPlaneHandler struct {
	crypto       primitives.KeyDeriver
	ackBuf       [udp.RouteIDLength + chacha20poly1305.NonceSize + service_packet.RekeyPacketLen + chacha20poly1305.Overhead]byte
	pongBuf      [udp.RouteIDLength + chacha20poly1305.NonceSize + 3 + chacha20poly1305.Overhead]byte
	exhaustedBuf [udp.RouteIDLength + chacha20poly1305.NonceSize + 3 + chacha20poly1305.Overhead]byte
}

func newServicePacketHandler(
	crypto primitives.KeyDeriver,
) controlPlaneHandler {
	return controlPlaneHandler{
		crypto: crypto,
	}
}

func (r *controlPlaneHandler) Handle(
	carrierEpoch uint16,
	plaindata []byte,
	egress connection.Egress,
	controller rekeyController,
) (bool, error) {
	if spType, ok := service_packet.TryParseHeader(plaindata); ok {
		switch spType {
		case service_packet.RekeyInit:
			return true, r.handleRekeyInit(carrierEpoch, plaindata, egress, controller)
		case service_packet.Ping:
			return true, r.handlePing(egress)
		default:
			return true, nil
		}
	}
	return false, nil
}

func (r *controlPlaneHandler) handlePing(egress connection.Egress) error {
	payloadOffset := udp.RouteIDLength + chacha20poly1305.NonceSize
	buf := r.pongBuf[:payloadOffset+3]
	payload := buf[payloadOffset:]
	if _, err := service_packet.EncodeV1Header(service_packet.Pong, payload); err != nil {
		return nil
	}
	_ = egress.SendControl(buf)
	return nil
}

func (r *controlPlaneHandler) handleRekeyInit(
	carrierEpoch uint16,
	plaindata []byte,
	egress connection.Egress,
	controller rekeyController,
) error {
	if controller == nil {
		return nil
	}
	serverPub, _, ok, err := controller.HandleRekeyInit(carrierEpoch, r.crypto, plaindata)
	if err != nil {
		if errors.Is(err, chacha20.ErrEpochExhausted) {
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
	payloadOffset := udp.RouteIDLength + chacha20poly1305.NonceSize
	ackBuf := r.ackBuf[:payloadOffset+service_packet.RekeyPacketLen]
	payload := ackBuf[payloadOffset:]
	copy(payload[3:], serverPub[:])
	if _, err = service_packet.EncodeV1Header(service_packet.RekeyAck, payload); err != nil {
		return nil
	}
	if err := egress.SendControl(ackBuf); err != nil {
		return nil
	}
	return nil
}

func (r *controlPlaneHandler) sendEpochExhausted(egress connection.Egress) {
	payloadOffset := udp.RouteIDLength + chacha20poly1305.NonceSize
	buf := r.exhaustedBuf[:payloadOffset+3]
	payload := buf[payloadOffset:]
	if _, err := service_packet.EncodeV1Header(service_packet.EpochExhausted, payload); err != nil {
		return
	}
	_ = egress.SendControl(buf)
}
