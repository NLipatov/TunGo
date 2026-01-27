package tcp_chacha20

import (
	"context"
	"io"
	"log"
	"time"
	"tungo/application/network/connection"
	"tungo/application/network/rekey"
	"tungo/application/network/routing/tun"
	"tungo/domain/network/service"
	"tungo/infrastructure/cryptography/chacha20/handshake"
	"tungo/infrastructure/settings"
)

type TunHandler struct {
	ctx                 context.Context
	reader              io.Reader // abstraction over TUN device
	writer              io.Writer // abstraction over transport
	cryptographyService connection.Crypto
	rekeyController     *rekey.Controller
	servicePacket       service.PacketHandler
	handshakeCrypto     handshake.Crypto
	rotateAt            time.Time
}

func NewTunHandler(ctx context.Context,
	reader io.Reader,
	writer io.Writer,
	cryptographyService connection.Crypto,
	rekeyController *rekey.Controller,
	servicePacket service.PacketHandler) tun.Handler {
	return &TunHandler{
		ctx:                 ctx,
		reader:              reader,
		writer:              writer,
		cryptographyService: cryptographyService,
		rekeyController:     rekeyController,
		servicePacket:       servicePacket,
		handshakeCrypto:     &handshake.DefaultCrypto{},
		rotateAt:            time.Now().UTC().Add(30 * time.Minute),
	}
}

func (t *TunHandler) HandleTun() error {
	// buffer has settings.TCPChacha20Overhead headroom for in-place encryption
	// payload itself will take settings.DefaultEthernetMTU bytes
	var buffer [settings.DefaultEthernetMTU + settings.TCPChacha20Overhead]byte
	payload := buffer[:settings.DefaultEthernetMTU]

	//passes anything from tun to chan
	for {
		select {
		case <-t.ctx.Done():
			return nil
		default:
			n, err := t.reader.Read(payload)
			if err != nil {
				if t.ctx.Err() != nil {
					return nil
				}
				log.Printf("failed to read from TUN: %v", err)
				return err
			}

			ciphertext, ciphertextErr := t.cryptographyService.Encrypt(payload[:n])
			if ciphertextErr != nil {
				log.Printf("failed to encrypt packet: %v", ciphertextErr)
				return ciphertextErr
			}

			_, err = t.writer.Write(ciphertext)
			if err != nil {
				log.Printf("write to TCP failed: %s", err)
				return err
			}

			if time.Now().UTC().After(t.rotateAt) && t.rekeyController != nil && t.rekeyController.State() == rekey.StateStable {
				pub, priv, keyErr := t.handshakeCrypto.GenerateX25519KeyPair()
				if keyErr != nil {
					log.Printf("failed to generate rekey key pair: %v", keyErr)
					t.rotateAt = time.Now().UTC().Add(30 * time.Minute)
					continue
				}
				t.rekeyController.SetPendingRekeyPrivateKey(priv)
				payloadBuf := make([]byte, service.RekeyPacketLen)
				copy(payloadBuf[3:], pub)
				servicePayload, err := t.servicePacket.EncodeV1(service.RekeyInit, payloadBuf)
				if err != nil {
					log.Printf("failed to encode rekeyInit packet")
					t.rotateAt = time.Now().UTC().Add(30 * time.Minute)
					continue
				}
				enc, encErr := t.cryptographyService.Encrypt(servicePayload)
				if encErr != nil {
					log.Printf("failed to encrypt rekeyInit: %v", encErr)
					t.rotateAt = time.Now().UTC().Add(30 * time.Minute)
					continue
				}
				if _, err := t.writer.Write(enc); err != nil {
					log.Printf("failed to write rekeyInit: %v", err)
				}
				t.rotateAt = time.Now().UTC().Add(30 * time.Minute)
			}
		}
	}
}
