package udp_chacha20

import (
	"errors"
	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20/handshake"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/network/service_packet"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
)

type servicePacketHandler struct {
	crypto handshake.Crypto
}

func newServicePacketHandler(
	crypto handshake.Crypto,
) servicePacketHandler {
	return servicePacketHandler{
		crypto: crypto,
	}
}

func (r *servicePacketHandler) Handle(
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

func (r *servicePacketHandler) handleRekeyInit(
	plaindata []byte,
	session connection.Session,
	fsm rekey.FSM,
) error {
	if fsm.State() != rekey.StateStable {
		return nil
	}
	if len(plaindata) < service_packet.RekeyPacketLen {
		// drop garbage
		return nil
	}
	var clientRekeyPub [service_packet.RekeyPublicKeyLen]byte
	copy(clientRekeyPub[:], plaindata[3:service_packet.RekeyPacketLen])

	serverPub, serverPriv, err := r.crypto.GenerateX25519KeyPair()
	if err != nil {
		return nil
	}
	shared, err := curve25519.X25519(serverPriv[:], clientRekeyPub[:])
	if err != nil {
		return nil
	}
	currentC2S := fsm.CurrentClientToServerKey()
	currentS2C := fsm.CurrentServerToClientKey()
	newC2S, err := r.crypto.DeriveKey(shared, currentC2S, []byte("tungo-rekey-c2s"))
	if err != nil {
		return nil
	}
	newS2C, err := r.crypto.DeriveKey(shared, currentS2C, []byte("tungo-rekey-s2c"))
	if err != nil {
		return nil
	}

	sendKey := newC2S
	recvKey := newS2C
	if fsm.IsServer() {
		sendKey, recvKey = newS2C, newC2S // server sends S2C, receives C2S
	}
	if _, err := fsm.StartRekey(sendKey, recvKey); err != nil {
		if errors.Is(err, rekey.ErrEpochExhausted) {
			return err
		}
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
