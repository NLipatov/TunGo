package udp_chacha20

import (
	"context"
	"golang.org/x/crypto/chacha20poly1305"
	"io"
	"log"
	"net/netip"
	"os"
	"tungo/application"
	"tungo/infrastructure/network"
	"tungo/infrastructure/routing/server_routing/session_management/repository"
)

type TunHandler struct {
	ctx            context.Context
	reader         io.Reader
	parser         network.IPHeader
	sessionManager repository.SessionRepository[application.Session]
}

func NewTunHandler(
	ctx context.Context,
	reader io.Reader,
	parser network.IPHeader,
	sessionManager repository.SessionRepository[application.Session],
) application.TunHandler {
	return &TunHandler{
		ctx:            ctx,
		reader:         reader,
		parser:         parser,
		sessionManager: sessionManager,
	}
}

func (t *TunHandler) HandleTun() error {
	buffer := make([]byte, network.MaxPacketLengthBytes+chacha20poly1305.NonceSize+chacha20poly1305.Overhead)
	destinationAddressBytes := [4]byte{}

	for {
		select {
		case <-t.ctx.Done():
			return nil
		default:
			// reserve first x bytes for nonce
			n, rErr := t.reader.Read(buffer[chacha20poly1305.NonceSize:])
			if rErr != nil {
				if t.ctx.Err() != nil {
					return nil
				}

				if rErr == io.EOF {
					log.Println("TUN interface closed, shutting down...")
					return rErr
				}

				if os.IsNotExist(rErr) || os.IsPermission(rErr) {
					log.Printf("TUN interface error (closed or permission issue): %v", rErr)
					return rErr
				}

				log.Printf("failed to read from TUN, retrying: %v", rErr)
				continue
			}

			if pErr := t.parser.ParseDestinationAddressBytes(
				buffer[chacha20poly1305.NonceSize:n+chacha20poly1305.NonceSize], destinationAddressBytes[:],
			); pErr != nil {
				log.Printf("packet dropped: header parsing error: %v", pErr)
				continue
			}

			addr, addrOk := netip.AddrFromSlice(destinationAddressBytes[:])
			if !addrOk {
				log.Printf(
					"packet dropped: failed to parse destination address bytes: %v", destinationAddressBytes[:])
				continue
			}

			session, sErr := t.sessionManager.GetByInternalAddrPort(addr)
			if sErr != nil {
				log.Printf("packet dropped: %s, destination host: %v", sErr, destinationAddressBytes)
				continue
			}

			encrypted, eErr := session.CryptographyService().Encrypt(buffer[:n+chacha20poly1305.NonceSize])
			if eErr != nil {
				log.Printf("failed to encrypt packet: %s", eErr)
				continue
			}

			if _, wErr := session.ConnectionAdapter().Write(encrypted); wErr != nil {
				log.Printf("failed to send packet to %v: %v", session.ExternalAddrPort(), wErr)
			}
		}
	}
}
