package tcp_chacha20

import (
	"context"
	"io"
	"log"
	"os"
	"tungo/application"
	appip "tungo/application/network/ip"
	"tungo/application/network/tun"
	"tungo/infrastructure/routing/server_routing/session_management/repository"
	"tungo/infrastructure/settings"
)

type TunHandler struct {
	ctx            context.Context
	reader         io.Reader
	ipHeaderParser appip.HeaderParser
	sessionManager repository.SessionRepository[application.Session]
}

func NewTunHandler(
	ctx context.Context,
	reader io.Reader,
	ipParser appip.HeaderParser,
	sessionManager repository.SessionRepository[application.Session],
) tun.Handler {
	return &TunHandler{
		ctx:            ctx,
		reader:         reader,
		ipHeaderParser: ipParser,
		sessionManager: sessionManager,
	}
}

func (t *TunHandler) HandleTun() error {
	backing := make([]byte, settings.DefaultEthernetMTU+settings.TCPChacha20Overhead)
	pt := backing[:settings.DefaultEthernetMTU]

	for {
		select {
		case <-t.ctx.Done():
			return nil
		default:
			n, err := t.reader.Read(pt)
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
			if n == 0 {
				// Defensive: spurious zero-length read; skip.
				continue
			}

			addr, addrErr := t.ipHeaderParser.DestinationAddress(pt[:n])
			if addrErr != nil {
				log.Printf("packet dropped: failed to parse destination address: %v", addrErr)
				continue
			}

			clientSession, getErr := t.sessionManager.GetByInternalAddrPort(addr)
			if getErr != nil {
				log.Printf("packet dropped: %s, destination host: %v", getErr, addr)
				continue
			}

			ct, encryptErr := clientSession.CryptographyService().Encrypt(pt[:n])
			if encryptErr != nil {
				log.Printf("failed to encrypt packet: %s", encryptErr)
				continue
			}

			_, connWriteErr := clientSession.ConnectionAdapter().Write(ct)
			if connWriteErr != nil {
				log.Printf("failed to write to TCP: %v", connWriteErr)
				t.sessionManager.Delete(clientSession)
			}
		}
	}
}
