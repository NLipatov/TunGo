package tcp_chacha20

import (
	"context"
	"golang.org/x/crypto/chacha20poly1305"
	"io"
	"log"
	"net/netip"
	"os"
	"tungo/application"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/network"
	"tungo/infrastructure/routing/server_routing/session_management/repository"
)

type TunHandler struct {
	ctx            context.Context
	reader         io.Reader
	encoder        chacha20.TCPEncoder
	ipParser       network.IPHeader
	sessionManager repository.SessionRepository[application.Session]
}

func NewTunHandler(
	ctx context.Context,
	reader io.Reader,
	encoder chacha20.TCPEncoder,
	ipParser network.IPHeader,
	sessionManager repository.SessionRepository[application.Session],
) application.TunHandler {
	return &TunHandler{
		ctx:            ctx,
		reader:         reader,
		encoder:        encoder,
		ipParser:       ipParser,
		sessionManager: sessionManager,
	}
}

func (t *TunHandler) HandleTun() error {
	buffer := make([]byte, 4+network.MaxPacketLengthBytes+chacha20poly1305.Overhead)
	destinationAddressBytes := [4]byte{}

	for {
		select {
		case <-t.ctx.Done():
			return nil
		default:
			// reserve 4 bytes for length prefix
			n, err := t.reader.Read(buffer[4:])
			if err != nil {
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

			payload := buffer[4 : n+4]

			destinationBytesErr := t.ipParser.ParseDestinationAddressBytes(payload, destinationAddressBytes[:])
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

			ct, encryptErr := clientSession.CryptographyService().Encrypt(payload)
			if encryptErr != nil {
				log.Printf("failed to encrypt packet: %s", encryptErr)
				continue
			}

			frame := buffer[:4+len(ct)]

			encodingErr := t.encoder.Encode(frame)
			if encodingErr != nil {
				log.Printf("failed to encode packet: %v", encodingErr)
				continue
			}

			_, connWriteErr := clientSession.ConnectionAdapter().Write(frame)
			if connWriteErr != nil {
				log.Printf("failed to write to TCP: %v", connWriteErr)
				t.sessionManager.Delete(clientSession)
			}
		}
	}
}
