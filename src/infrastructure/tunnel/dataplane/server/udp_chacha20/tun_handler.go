package udp_chacha20

import (
	"context"
	"io"
	"log"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/settings"

	"golang.org/x/crypto/chacha20poly1305"

	appip "tungo/application/network/ip"
	"tungo/infrastructure/tunnel/session"
)

type TunHandler struct {
	ctx            context.Context
	reader         io.Reader
	ipHeaderParser appip.HeaderParser
	sessionManager session.Repository
}

func NewTunHandler(
	ctx context.Context,
	reader io.Reader,
	parser appip.HeaderParser,
	sessionManager session.Repository,
) tun.Handler {
	return &TunHandler{
		ctx:            ctx,
		reader:         reader,
		ipHeaderParser: parser,
		sessionManager: sessionManager,
	}
}

// HandleTun reads packets from the TUN interface,
// reserves space for AEAD overhead, encrypts them, and forwards them to the correct session.
//
// Buffer layout before Encrypt (total size = MTU + UDPChacha20Overhead):
//
//	[ 0 ........ 11 ][ 12 ........ 1511 ][ 1512 ........ end ]
//	|   Nonce    |      Payload (<= MTU) |   spare headroom   |
//
// Example with settings.MTU = 1500, settings.UDPChacha20Overhead = 36:
// - buffer length = 1500 + 36 = 1536
//
// Step 1 – read plaintext from TUN:
// - reader.Read writes at most MTU bytes into buffer[12:1512].
// - the first 12 bytes (buffer[0:12]) are reserved for the nonce
// - trailing headroom is used by Encrypt for Poly1305 tag (+16) and route-id prefix (+8)
//
// Step 2 – encrypt plaintext in place:
//   - encryption operates on buffer[0 : 12+n] (nonce + payload)
//   - ciphertext and authentication tag are written back in place
//   - no additional allocations are required since both the prefix
//     (nonce) and the suffix (tag) are already reserved in the buffer.
func (t *TunHandler) HandleTun() error {
	// Reserve space for nonce + payload + AEAD tag (in-place encryption needs extra capacity).
	var buffer [settings.DefaultEthernetMTU + settings.UDPChacha20Overhead]byte
	plaintext := buffer[chacha20poly1305.NonceSize : settings.DefaultEthernetMTU+chacha20poly1305.NonceSize]

	for {
		select {
		case <-t.ctx.Done():
			return nil
		default:
			// Read payload right after the reserved nonce area.
			n, err := t.reader.Read(plaintext)
			if err != nil {
				if ne, ok := err.(interface{ Temporary() bool }); ok && ne.Temporary() {
					continue
				}
				return err
			}
			if n == 0 {
				// Defensive: spurious zero-length read; skip.
				continue
			}

			// Parse destination from the IP header (skip the nonce).
			payload := buffer[chacha20poly1305.NonceSize : chacha20poly1305.NonceSize+n]
			addr, addrErr := t.ipHeaderParser.DestinationAddress(payload)
			if addrErr != nil {
				log.Printf("packet dropped: failed to parse destination address: %v", addrErr)
				continue
			}

			peer, sErr := t.sessionManager.FindByDestinationIP(addr)
			if sErr != nil {
				// No route to destination - either unknown host or not in any peer's AllowedIPs
				continue
			}

			// Encrypt "nonce || payload". The crypto service_packet must treat the prefix as nonce.
			if err := peer.Egress().SendDataIP(buffer[:chacha20poly1305.NonceSize+n]); err != nil {
				log.Printf("failed to send packet to %v: %v", peer.ExternalAddrPort(), err)
				t.sessionManager.Delete(peer)
			}
		}
	}
}
