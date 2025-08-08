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
			// reserve first 12 bytes for encryption overhead (12 bytes nonce)
			n, err := t.reader.Read(buffer[12:])
			if err != nil {
				if t.ctx.Err() != nil {
					return nil
				}

				if err == io.EOF {
					log.Println("TUN interface closed, shutting down...")
					return err
				}

				if os.IsNotExist(err) || os.IsPermission(err) {
					log.Printf("TUN interface error (closed or permission issue): %v", err)
					return err
				}

				log.Printf("failed to read from TUN, retrying: %v", err)
				continue
			}

			destinationBytesErr := t.parser.ParseDestinationAddressBytes(buffer[12:n+12], destinationAddressBytes[:])
			if destinationBytesErr != nil {
				log.Printf("packet dropped: header parsing error: %v", destinationBytesErr)
				continue
			}

			destAddr, destAddrOk := netip.AddrFromSlice(destinationAddressBytes[:])
			if !destAddrOk {
				log.Printf(
					"packet dropped: failed to parse destination address bytes: %v", destinationAddressBytes[:])
				continue
			}

			clientSession, getErr := t.sessionManager.GetByInternalAddrPort(destAddr)
			if getErr != nil {
				log.Printf("packet dropped: %s, destination host: %v", getErr, destinationAddressBytes)
				continue
			}

			encryptedPacket, encryptErr := clientSession.CryptographyService().Encrypt(buffer[:n+12])
			if encryptErr != nil {
				log.Printf("failed to encrypt packet: %s", encryptErr)
				continue
			}

			_, writeToUDPErr := clientSession.ConnectionAdapter().Write(encryptedPacket)
			if writeToUDPErr != nil {
				log.Printf("failed to send packet to %v: %v", clientSession.ExternalAddrPort(), writeToUDPErr)
			}
		}
	}
}
