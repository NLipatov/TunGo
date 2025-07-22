package tcp_chacha20

import (
	"context"
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
	sessionManager repository.SessionRepository[Session]
}

func NewTunHandler(
	ctx context.Context,
	reader io.Reader,
	encoder chacha20.TCPEncoder,
	ipParser network.IPHeader,
	sessionManager repository.SessionRepository[Session],
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
	buf := make([]byte, network.MaxPacketLengthBytes)
	destinationAddressBytes := [4]byte{}

	for {
		select {
		case <-t.ctx.Done():
			return nil
		default:
			n, err := t.reader.Read(buf)
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
			if n <= 4 {
				log.Printf("invalid IP data (n=%d < 4)", n)
				continue
			}

			data := buf[:n]
			destinationBytesErr := t.ipParser.ParseDestinationAddressBytes(data, destinationAddressBytes[:])
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

			encrypted, encryptErr := clientSession.CryptographyService.Encrypt(data)
			if encryptErr != nil {
				log.Printf("failed to encrypt packet: %s", encryptErr)
				continue
			}

			adapter := chacha20.NewClientTCPAdapter(clientSession.conn)
			_, connWriteErr := adapter.Write(encrypted)
			if connWriteErr != nil {
				log.Printf("failed to write to TCP: %v", connWriteErr)
				t.sessionManager.Delete(clientSession)
			}
		}
	}
}
