package udp_chacha20

import (
	"context"
	"io"
	"log"
	"net/netip"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/settings"

	"golang.org/x/crypto/chacha20poly1305"

	appip "tungo/application/network/ip"
	"tungo/infrastructure/tunnel/session"
)

type TunHandler struct {
	ctx              context.Context
	reader           io.Reader
	ipHeaderParser   appip.HeaderParser
	sessionManager   session.Repository
	sendSessionReset func(netip.AddrPort)
}

func NewTunHandler(
	ctx context.Context,
	reader io.Reader,
	parser appip.HeaderParser,
	sessionManager session.Repository,
	sendSessionReset func(netip.AddrPort),
) tun.Handler {
	return &TunHandler{
		ctx:              ctx,
		reader:           reader,
		ipHeaderParser:   parser,
		sessionManager:   sessionManager,
		sendSessionReset: sendSessionReset,
	}
}

// HandleTun reads packets from the TUN interface,
// reserves space for AEAD overhead, encrypts them, and forwards them to the correct session.
//
// Buffer layout (total size = MTU + NonceSize + TagSize):
//
//	[ 0 ........ 11 ][ 12 ........ 1511 ][ 1512 ........ 1527 ]
//	|   Nonce    |      Payload (<= MTU) |       Tag (16B)    |
//
// Example with settings.MTU = 1500, settings.UDPChacha20Overhead = 28:
// - buffer length = 1500 + 28 = 1528
//
// Step 1 – read plaintext from TUN:
// - reader.Read writes at most MTU bytes into buffer[12:1512].
// - the first 12 bytes (buffer[0:12]) are reserved for the nonce
// - the last 16 bytes (buffer[1512:1528]) are reserved for the Poly1305 tag
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

			peer, sErr := t.sessionManager.GetByInternalAddrPort(addr)
			if sErr != nil {
				log.Printf("packet dropped: %v, destination host: %v", sErr, addr)
				continue
			}

			// Encrypt "nonce || payload". The crypto service_packet must treat the prefix as nonce.
			if err := peer.Egress().SendDataIP(buffer[:chacha20poly1305.NonceSize+n]); err != nil {
				log.Printf("failed to send packet to %v: %v", peer.ExternalAddrPort(), err)
				t.sendSessionReset(peer.ExternalAddrPort())
				t.sessionManager.Delete(peer)
			}
		}
	}
}
