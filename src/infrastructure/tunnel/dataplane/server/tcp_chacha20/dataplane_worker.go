package tcp_chacha20

import (
	"context"
	"io"

	"tungo/application/logging"
	"tungo/application/network/connection"
	"tungo/infrastructure/network/ip"
	"tungo/infrastructure/network/service_packet"
	"tungo/infrastructure/settings"
	"tungo/infrastructure/tunnel/session"

	"golang.org/x/crypto/chacha20poly1305"
)

// tcpDataplaneWorker runs a per-connection dataplane loop:
// read ciphertext -> decrypt -> (controlplane dispatch | write to TUN).
type tcpDataplaneWorker struct {
	ctx            context.Context
	peer           *session.Peer
	transport      connection.Transport
	tunFile        io.ReadWriteCloser
	sessionManager session.Repository
	logger         logging.Logger
	cp             controlPlaneHandler
}

func (w *tcpDataplaneWorker) Run() {
	defer func() {
		w.sessionManager.Delete(w.peer)
		_ = w.transport.Close()
		w.logger.Printf("disconnected: %s", w.peer.ExternalAddrPort())
	}()

	buffer := make([]byte, settings.DefaultEthernetMTU+settings.TCPChacha20Overhead)
	for {
		select {
		case <-w.ctx.Done():
			return
		default:
			n, err := w.transport.Read(buffer)
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
			// SECURITY: Check closed flag before using crypto.
			// ConfigWatcher may have terminated this session via TerminateByPubKey.
			// The closed flag is set atomically before crypto is zeroed.
			if w.peer.IsClosed() {
				w.logger.Printf("session closed, exiting")
				return
			}
			pt, err := w.peer.Crypto().Decrypt(buffer[:n])
			if err != nil {
				w.logger.Printf("failed to decrypt data: %s", err)
				return
			}
			if rc := w.peer.RekeyController(); rc != nil {
				if spType, spOk := service_packet.TryParseHeader(pt); spOk {
					if spType == service_packet.RekeyInit {
						w.cp.Handle(pt, w.peer.Egress(), rc)
						continue
					}
					// server ignores Ack
				}
			}

			// Validate source IP against AllowedIPs after decryption
			// Session interface embeds SessionAuth - no type assertion needed
			srcIP, srcOk := ip.ExtractSourceIP(pt)
			if !srcOk {
				// Malformed IP header - drop to prevent AllowedIPs bypass
				continue
			}
			if !w.peer.Session.IsSourceAllowed(srcIP) {
				// Log violation and drop packet, but do NOT terminate session
				w.logger.Printf("AllowedIPs violation: source %s not allowed", srcIP)
				continue
			}

			if _, err = w.tunFile.Write(pt); err != nil {
				w.logger.Printf("failed to write to TUN: %v", err)
				return
			}
		}
	}
}
