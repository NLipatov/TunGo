package tcp_chacha20

import (
	"context"
	"io"
	"log"
	appip "tungo/application/network/ip"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/settings"
	"tungo/infrastructure/telemetry/trafficstats"
	"tungo/infrastructure/tunnel/session"
)

// epochPrefixSize is the number of bytes reserved at the start of the buffer
// for the 2-byte epoch tag prepended to every TCP ciphertext frame.
const epochPrefixSize = 2

type TunHandler struct {
	ctx            context.Context
	reader         io.Reader
	ipHeaderParser appip.HeaderParser
	peerStore      session.PeerStore
}

func NewTunHandler(
	ctx context.Context,
	reader io.Reader,
	ipParser appip.HeaderParser,
	peerStore session.PeerStore,
) tun.Handler {
	return &TunHandler{
		ctx:            ctx,
		reader:         reader,
		ipHeaderParser: ipParser,
		peerStore:      peerStore,
	}
}

func (t *TunHandler) HandleTun() error {
	// Buffer layout: [2B epoch reserved][plaintext up to MTU][16B AEAD tag capacity]
	var buffer [settings.DefaultEthernetMTU + settings.TCPChacha20Overhead]byte
	plaintext := buffer[epochPrefixSize : settings.DefaultEthernetMTU+epochPrefixSize]
	rec := trafficstats.NewRecorder()
	defer rec.Flush()

	for {
		select {
		case <-t.ctx.Done():
			return nil
		default:
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

			addr, addrErr := t.ipHeaderParser.DestinationAddress(plaintext[:n])
			if addrErr != nil {
				log.Printf("packet dropped: failed to parse destination address: %v", addrErr)
				continue
			}

			peer, getErr := t.peerStore.FindByDestinationIP(addr)
			if getErr != nil {
				// No route to destination - either unknown host or not in any peer's AllowedIPs
				continue
			}

			// Pass buffer including the 2-byte epoch prefix reservation.
			if err := peer.Egress().SendDataIP(buffer[:epochPrefixSize+n]); err != nil {
				log.Printf("failed to write to TCP: %v", err)
				_ = peer.Egress().Close()
				t.peerStore.Delete(peer)
			}
			rec.RecordTX(uint64(n))
		}
	}
}
