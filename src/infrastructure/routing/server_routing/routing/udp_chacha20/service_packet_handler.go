package udp_chacha20

import (
	"errors"
	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20/handshake"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/network/service_packet"
	"tungo/infrastructure/routing/controlplane"

	"golang.org/x/crypto/chacha20poly1305"
)

// controlPlaneHandler is a dataplane-adapter for inbound control-plane packets.
// It delegates protocol logic to infrastructure/routing/controlplane.
type controlPlaneHandler struct {
	crypto handshake.Crypto
}

func newServicePacketHandler(
	crypto handshake.Crypto,
) controlPlaneHandler {
	return controlPlaneHandler{
		crypto: crypto,
	}
}

func (r *controlPlaneHandler) Handle(
	plaindata []byte,
	session connection.Session,
	fsm rekey.FSM,
) (bool, error) {
	if spType, ok := service_packet.TryParseHeader(plaindata); ok {
		switch spType {
		case service_packet.RekeyInit:
			return true, r.handleRekeyInit(plaindata, session, fsm)
		default:
			return true, nil
		}
	}
	return false, nil
}

func (r *controlPlaneHandler) handleRekeyInit(
	plaindata []byte,
	session connection.Session,
	fsm rekey.FSM,
) error {
	serverPub, _, ok, err := controlplane.ServerHandleRekeyInit(r.crypto, fsm, plaindata)
	if err != nil {
		if errors.Is(err, rekey.ErrEpochExhausted) {
			return err
		}
		return nil
	}
	if !ok {
		return nil
	}
	// Only send ACK after successful rekey installation.
	ackBuf := make([]byte, chacha20poly1305.NonceSize+service_packet.RekeyPacketLen,
		chacha20poly1305.NonceSize+service_packet.RekeyPacketLen+chacha20poly1305.Overhead)
	payload := ackBuf[chacha20poly1305.NonceSize:]
	copy(payload[3:], serverPub)
	if _, err = service_packet.EncodeV1Header(service_packet.RekeyAck, payload); err != nil {
		return nil
	}
	if err := session.Outbound().SendControl(ackBuf); err != nil {
		return nil
	}
	return nil
}
