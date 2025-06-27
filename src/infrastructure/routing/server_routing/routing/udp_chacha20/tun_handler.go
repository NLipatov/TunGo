package udp_chacha20

import (
	"context"
	"io"
	"log"
	"os"
	"tungo/application"
	"tungo/infrastructure/network"
	"tungo/infrastructure/routing/server_routing/session_management"
)

type TunHandler struct {
	ctx            context.Context
	reader         io.Reader
	parser         network.IPHeader
	sessionManager session_management.WorkerSessionManager[Session]
}

func NewTunHandler(
	ctx context.Context,
	reader io.Reader,
	parser network.IPHeader,
	sessionManager session_management.WorkerSessionManager[Session],
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

			clientSession, getErr := t.sessionManager.GetByInternalIP(destinationAddressBytes)
			if getErr != nil {
				log.Printf("packet dropped: %s, destination host: %v", getErr, destinationAddressBytes)
				continue
			}

			encryptedPacket, encryptErr := clientSession.CryptographyService.Encrypt(packetBuffer)
			if encryptErr != nil {
				log.Printf("failed to encrypt packet: %s", encryptErr)
				continue
			}

			_, writeToUDPErr := clientSession.connectionAdapter.Write(encryptedPacket)
			if writeToUDPErr != nil {
				log.Printf("failed to send packet to %v: %v", clientSession.remoteAddrPort, writeToUDPErr)
			}
		}
	}
}
