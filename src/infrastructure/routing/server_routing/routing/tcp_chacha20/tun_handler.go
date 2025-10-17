package tcp_chacha20

import (
	"context"
	"io"
	"log"
	"tungo/application/network/connection"
	appip "tungo/application/network/ip"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/routing/server_routing/session_management/repository"
	"tungo/infrastructure/settings"
)

type TunHandler struct {
	ctx            context.Context
	reader         io.Reader
	ipHeaderParser appip.HeaderParser
	sessionManager repository.SessionRepository[connection.Session]
}

func NewTunHandler(
	ctx context.Context,
	reader io.Reader,
	ipParser appip.HeaderParser,
	sessionManager repository.SessionRepository[connection.Session],
) tun.Handler {
	return &TunHandler{
		ctx:            ctx,
		reader:         reader,
		ipHeaderParser: ipParser,
		sessionManager: sessionManager,
	}
}

func (t *TunHandler) HandleTun() error {
	var buffer [settings.DefaultEthernetMTU + settings.TCPChacha20Overhead]byte
	plaintext := buffer[:settings.DefaultEthernetMTU]

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

			clientSession, getErr := t.sessionManager.GetByInternalAddrPort(addr)
			if getErr != nil {
				log.Printf("packet dropped: %s, destination host: %v", getErr, addr)
				continue
			}

			ct, encryptErr := clientSession.Crypto().Encrypt(plaintext[:n])
			if encryptErr != nil {
				log.Printf("failed to encrypt packet: %s", encryptErr)
				continue
			}

			_, connWriteErr := clientSession.Transport().Write(ct)
			if connWriteErr != nil {
				log.Printf("failed to write to TCP: %v", connWriteErr)
				t.sessionManager.Delete(clientSession)
			}
		}
	}
}
