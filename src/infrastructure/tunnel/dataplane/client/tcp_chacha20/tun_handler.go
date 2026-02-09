package tcp_chacha20

import (
	"context"
	"io"
	"log"
	"time"
	"tungo/application/network/connection"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/network/service_packet"
	"tungo/infrastructure/settings"
	"tungo/infrastructure/tunnel/controlplane"
)

// epochPrefixSize is the number of bytes reserved at the start of the buffer
// for the 2-byte epoch tag prepended to every TCP ciphertext frame.
const epochPrefixSize = 2

type TunHandler struct {
	ctx              context.Context
	reader           io.Reader // abstraction over TUN device
	egress           connection.Egress
	rekeyController  *rekey.StateMachine
	rekeyInit        *controlplane.RekeyInitScheduler
	controlPacketBuf [epochPrefixSize + service_packet.RekeyPacketLen + settings.TCPChacha20Overhead]byte
}

func NewTunHandler(ctx context.Context,
	reader io.Reader,
	egress connection.Egress,
	rekeyController *rekey.StateMachine,
) tun.Handler {
	now := time.Now().UTC()
	return &TunHandler{
		ctx:             ctx,
		reader:          reader,
		egress:          egress,
		rekeyController: rekeyController,
		rekeyInit:       controlplane.NewRekeyInitScheduler(&primitives.DefaultKeyDeriver{}, settings.DefaultRekeyInterval, now),
	}
}

func (t *TunHandler) HandleTun() error {
	// Buffer layout: [2B epoch reserved][plaintext up to MTU][16B AEAD tag capacity]
	var buffer [settings.DefaultEthernetMTU + settings.TCPChacha20Overhead]byte
	payload := buffer[epochPrefixSize : settings.DefaultEthernetMTU+epochPrefixSize]

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

			// Pass buffer including the 2-byte epoch prefix reservation.
			if err := t.egress.SendDataIP(buffer[:epochPrefixSize+n]); err != nil {
				log.Printf("write to TCP failed: %s", err)
				return err
			}

			if t.rekeyInit != nil && t.rekeyController != nil {
				now := time.Now().UTC()
				dst := t.controlPacketBuf[epochPrefixSize : epochPrefixSize+service_packet.RekeyPacketLen]
				servicePayload, ok, err := t.rekeyInit.MaybeBuildRekeyInit(now, t.rekeyController, dst)
				if err != nil {
					log.Printf("failed to prepare rekeyInit: %v", err)
					continue
				}
				if ok {
					spWithPrefix := t.controlPacketBuf[:epochPrefixSize+len(servicePayload)]
					if err := t.egress.SendControl(spWithPrefix); err != nil {
						log.Printf("failed to send rekeyInit: %v", err)
					}
				}
			}
		}
	}
}
