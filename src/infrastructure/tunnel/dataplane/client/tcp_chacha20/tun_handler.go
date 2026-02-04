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

type TunHandler struct {
	ctx                 context.Context
	reader              io.Reader // abstraction over TUN device
	writer              io.Writer // abstraction over transport
	cryptographyService connection.Crypto
	egress              connection.Egress
	rekeyController     *rekey.StateMachine
	rekeyInit           *controlplane.RekeyInitScheduler
	controlPacketBuf    [service_packet.RekeyPacketLen + settings.TCPChacha20Overhead]byte
}

func NewTunHandler(ctx context.Context,
	reader io.Reader,
	writer io.Writer,
	cryptographyService connection.Crypto,
	rekeyController *rekey.StateMachine,
) tun.Handler {
	now := time.Now().UTC()
	return &TunHandler{
		ctx:                 ctx,
		reader:              reader,
		writer:              writer,
		cryptographyService: cryptographyService,
		egress:              connection.NewDefaultEgress(writer, cryptographyService),
		rekeyController:     rekeyController,
		rekeyInit:           controlplane.NewRekeyInitScheduler(&primitives.DefaultKeyDeriver{}, settings.DefaultRekeyInterval, now),
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

			if err := t.egress.SendDataIP(payload[:n]); err != nil {
				log.Printf("write to TCP failed: %s", err)
				return err
			}

			if t.rekeyInit != nil && t.rekeyController != nil {
				now := time.Now().UTC()
				dst := t.controlPacketBuf[:service_packet.RekeyPacketLen]
				servicePayload, ok, err := t.rekeyInit.MaybeBuildRekeyInit(now, t.rekeyController, dst)
				if err != nil {
					log.Printf("failed to prepare rekeyInit: %v", err)
					continue
				}
				if ok {
					if err := t.egress.SendControl(servicePayload); err != nil {
						log.Printf("failed to send rekeyInit: %v", err)
					}
				}
			}
		}
	}
}
