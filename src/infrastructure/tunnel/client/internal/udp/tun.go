package udp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"time"

	"tungo/application/configuration/settings"
	udpcrypto "tungo/infrastructure/cryptography/chacha20/udp"
	"tungo/infrastructure/network/ip"
)

const udpPayloadOffset = udpcrypto.PayloadOffset

type rekeyInitiator interface {
	MaybeBuildRekeyInit(now time.Time, dst []byte) (payload []byte, ok bool, err error)
}

type tunHandler struct {
	ctx                 context.Context
	reader              io.Reader // abstraction over TUN device
	egress              sender
	allowedSources      map[netip.Addr]struct{}
	controlPacketBuffer [128]byte
	rekeyInit           rekeyInitiator
}

func newTunHandler(ctx context.Context,
	reader io.Reader,
	egress sender,
	rekeyInit rekeyInitiator,
	allowedSources map[netip.Addr]struct{},
) *tunHandler {
	return &tunHandler{
		ctx:            ctx,
		reader:         reader,
		egress:         egress,
		rekeyInit:      rekeyInit,
		allowedSources: allowedSources,
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
// - first 20 bytes are reserved for Route ID and nonce
// - trailing headroom is used by Encrypt for Poly1305 tag (+16)
//
// Step 2 – encrypt plaintext in place:
//   - encryption operates on buffer[0 : 20+n] (Route ID + nonce + payload)
//   - ciphertext and authentication tag are written back in place
//   - no additional allocations are required since all prefixes and suffix headroom are reserved.
func (w *tunHandler) HandleTun() error {
	// +8 Route ID +12 nonce +16 AEAD tag
	var buffer [settings.DefaultEthernetMTU + settings.UDPChacha20Overhead]byte
	payloadStart := udpPayloadOffset

	// Main loop to read from TUN and send data
	for {
		select {
		case <-w.ctx.Done():
			return nil
		default:
			n, err := w.reader.Read(buffer[payloadStart : payloadStart+settings.DefaultEthernetMTU])
			if n > 0 && len(w.allowedSources) > 0 && !ip.IsAllowedSource(buffer[payloadStart:payloadStart+n], w.allowedSources) {
				n = 0 // drop; fall through to error check
			}
			if n > 0 {
				// Encrypt expects Route ID + nonce + payload (20+n).
				if err := w.egress.Send(buffer[:payloadStart+n]); err != nil {
					if w.ctx.Err() != nil {
						return nil
					}
					if _, ok := errors.AsType[net.Error](err); ok {
						// Transient socket error (e.g. WSAENOBUFS) — packet lost, socket is fine.
						slog.Warn("transient write error, packet dropped", "err", err)
						continue
					}
					return fmt.Errorf("could not send packet to transport: %v", err)
				}
			}
			if err != nil {
				if w.ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("could not read a packet from TUN: %v", err)
			}
			if w.rekeyInit != nil {
				payloadBuf := w.controlPacketBuffer[udpPayloadOffset:]
				servicePayload, ok, pErr := w.rekeyInit.MaybeBuildRekeyInit(time.Now().UTC(), payloadBuf)
				if pErr != nil {
					slog.Warn("failed to prepare rekey init", "err", pErr)
					continue
				}
				if ok {
					totalLen := udpPayloadOffset + len(servicePayload)
					if err := w.egress.Send(w.controlPacketBuffer[:totalLen]); err != nil {
						slog.Warn("failed to send rekey init", "err", err)
					}
				}
			}
		}
	}
}
