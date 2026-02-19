package udp_chacha20

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/netip"
	"time"
	"tungo/application/network/connection"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/network/ip"
	"tungo/infrastructure/settings"
	"tungo/infrastructure/telemetry/trafficstats"
	"tungo/infrastructure/tunnel/controlplane"

	"golang.org/x/crypto/chacha20poly1305"
)

type TunHandler struct {
	ctx                 context.Context
	reader              io.Reader // abstraction over TUN device
	egress              connection.Egress
	rekeyController     *rekey.StateMachine
	allowedSources      map[netip.Addr]struct{}
	controlPacketBuffer [128]byte
	rekeyInit           *controlplane.RekeyInitScheduler
}

func NewTunHandler(ctx context.Context,
	reader io.Reader,
	egress connection.Egress,
	rekeyController *rekey.StateMachine,
	allowedSources map[netip.Addr]struct{},
) tun.Handler {
	now := time.Now().UTC()
	return &TunHandler{
		ctx:             ctx,
		reader:          reader,
		egress:          egress,
		rekeyController: rekeyController,
		rekeyInit:       controlplane.NewRekeyInitScheduler(&primitives.DefaultKeyDeriver{}, settings.DefaultRekeyInterval, now),
		allowedSources:  allowedSources,
	}
}

// HandleTun reads packets from the TUN interface,
// reserves space for AEAD overhead, encrypts them, and forwards them to the correct session.
//
// Buffer layout before Encrypt (total size = MTU + UDPChacha20Overhead):
//
//	[ 0 ..... 7 ][ 8 .... 19 ][ 20 ........ 1519 ][ 1520 ..... end ]
//	| Route ID  |   Nonce    |   Payload (<= MTU) |   AEAD tag headroom |
//
// Example with MTU = 1500, settings.UDPChacha20Overhead = 36:
// - buffer length = 1500 + 36 = 1536
//
// Step 1 – read plaintext from TUN:
// - reader.Read writes at most MTU bytes into buffer[20:1520].
// - first 8 bytes are reserved for route-id, next 12 for nonce
// - trailing headroom is used by Encrypt for Poly1305 tag (+16)
//
// Step 2 – encrypt plaintext in place:
//   - encryption operates on buffer[0 : 20+n] (route-id + nonce + payload)
//   - ciphertext and authentication tag are written back in place
//   - no additional allocations are required since all prefixes and suffix headroom are reserved.
func (w *TunHandler) HandleTun() error {
	// +8 route-id +12 nonce +16 AEAD tag
	var buffer [settings.DefaultEthernetMTU + settings.UDPChacha20Overhead]byte
	payloadStart := chacha20.UDPRouteIDLength + chacha20poly1305.NonceSize
	rec := trafficstats.NewRecorder()

	// Main loop to read from TUN and send data
	for {
		select {
		case <-w.ctx.Done():
			rec.Flush()
			return nil
		default:
			n, err := w.reader.Read(buffer[payloadStart : payloadStart+settings.DefaultEthernetMTU])
			if n > 0 && len(w.allowedSources) > 0 && !ip.IsAllowedSource(buffer[payloadStart:payloadStart+n], w.allowedSources) {
				n = 0 // drop; fall through to error check
			}
			if n > 0 {
				// Encrypt expects route-id+nonce+payload (20+n).
				if err := w.egress.SendDataIP(buffer[:payloadStart+n]); err != nil {
					if w.ctx.Err() != nil {
						rec.Flush()
						return nil
					}
					rec.Flush()
					return fmt.Errorf("could not send packet to transport: %v", err)
				}
				rec.RecordTX(uint64(n))
			}
			if err != nil {
				if w.ctx.Err() != nil {
					rec.Flush()
					return nil
				}
				rec.Flush()
				return fmt.Errorf("could not read a packet from TUN: %v", err)
			}
			if w.rekeyInit != nil && w.rekeyController != nil {
				payloadBuf := w.controlPacketBuffer[chacha20.UDPRouteIDLength+chacha20poly1305.NonceSize:]
				servicePayload, ok, pErr := w.rekeyInit.MaybeBuildRekeyInit(time.Now().UTC(), w.rekeyController, payloadBuf)
				if pErr != nil {
					log.Printf("failed to prepare rekeyInit: %v", pErr)
					continue
				}
				if ok {
					totalLen := chacha20.UDPRouteIDLength + chacha20poly1305.NonceSize + len(servicePayload)
					if err := w.egress.SendControl(w.controlPacketBuffer[:totalLen]); err != nil {
						log.Printf("failed to send rekeyInit: %v", err)
					}
				}
			}
		}
	}
}
