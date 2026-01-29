package tcp_chacha20

import (
	"context"
	"io"

	"tungo/application/logging"
	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/network/service_packet"
	"tungo/infrastructure/routing/server_routing/session_management/repository"
	"tungo/infrastructure/settings"

	"golang.org/x/crypto/chacha20poly1305"
)

// tcpDataplaneWorker runs a per-connection dataplane loop:
// read ciphertext -> decrypt -> (controlplane dispatch | write to TUN).
type tcpDataplaneWorker struct {
	ctx            context.Context
	session        connection.Session
	tunFile        io.ReadWriteCloser
	sessionManager repository.SessionRepository[connection.Session]
	logger         logging.Logger
	onRekeyInit    func(fsm rekey.FSM, session connection.Session, pt []byte)
}

func (w *tcpDataplaneWorker) Run() {
	defer func() {
		w.sessionManager.Delete(w.session)
		_ = w.session.Transport().Close()
		w.logger.Printf("disconnected: %s", w.session.ExternalAddrPort())
	}()

	buffer := make([]byte, settings.DefaultEthernetMTU+settings.TCPChacha20Overhead)
	for {
		select {
		case <-w.ctx.Done():
			return
		default:
			n, err := w.session.Transport().Read(buffer)
			if err != nil {
				if err != io.EOF {
					w.logger.Printf("failed to read from client: %v", err)
				}
				return
			}
			if n < chacha20poly1305.Overhead || n > settings.DefaultEthernetMTU+settings.TCPChacha20Overhead {
				w.logger.Printf("invalid ciphertext length: %d", n)
				continue
			}
			pt, err := w.session.Crypto().Decrypt(buffer[:n])
			if err != nil {
				w.logger.Printf("failed to decrypt data: %s", err)
				continue
			}
			if rc := w.session.RekeyController(); rc != nil {
				if spType, spOk := service_packet.TryParseHeader(pt); spOk {
					if spType == service_packet.RekeyInit && w.onRekeyInit != nil {
						w.onRekeyInit(rc, w.session, pt)
						continue
					}
					// server ignores Ack
				}
			}
			if _, err = w.tunFile.Write(pt); err != nil {
				w.logger.Printf("failed to write to TUN: %v", err)
				return
			}
		}
	}
}
