package udp_chacha20

import (
	"context"
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
	packetBuffer := make([]byte, network.MaxPacketLengthBytes+12)
	destinationAddressBytes := [4]byte{}

	for {
		select {
		case <-t.ctx.Done():
			return nil
		default:
			n, err := t.reader.Read(packetBuffer)
			if err != nil {
				if t.ctx.Done() != nil {
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

			if n < 12 {
				log.Printf("invalid packet length (%d < 12)", n)
				continue
			}

			// see udp_reader.go. It's putting payload length into first 12 bytes.
			payload := packetBuffer[12 : n+12]
			destinationBytesErr := t.parser.ParseDestinationAddressBytes(payload, destinationAddressBytes[:])
			if destinationBytesErr != nil {
				log.Printf("packet dropped: failed to read destination address bytes: %v", destinationBytesErr)
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

			encryptedPacket, encryptErr := clientSession.CryptographyService().Encrypt(packetBuffer)
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
