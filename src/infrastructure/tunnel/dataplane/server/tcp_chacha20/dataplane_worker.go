package tcp_chacha20

import (
	"context"
	"io"

	"tungo/application/logging"
	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/primitives"
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

func newTCPDataplaneWorker(
	ctx context.Context,
	peer *session.Peer,
	transport connection.Transport,
	tunFile io.ReadWriteCloser,
	sessionManager session.Repository,
	logger logging.Logger,
) *tcpDataplaneWorker {
	crypto := &primitives.DefaultKeyDeriver{}
	return &tcpDataplaneWorker{
		ctx:            ctx,
		peer:           peer,
		transport:      transport,
		tunFile:        tunFile,
		sessionManager: sessionManager,
		logger:         logger,
		cp:             newControlPlaneHandler(crypto, logger),
	}
}

func (w *tcpDataplaneWorker) Run() {
	defer func() {
		w.sessionManager.Delete(w.peer)
		_ = w.transport.Close()
		w.logger.Printf("disconnected: %s", w.peer.ExternalAddrPort())
	}()

	var buffer [settings.DefaultEthernetMTU + settings.TCPChacha20Overhead]byte
	for {
		select {
		case <-w.ctx.Done():
			return
		default:
			n, err := w.transport.Read(buffer[:])
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
			// SECURITY: Acquire crypto read lock before decryption.
			// This prevents the TOCTOU race where ConfigWatcher or idle reaper
			// could zeroize crypto between closed check and Decrypt call.
			if !w.peer.CryptoRLock() {
				w.logger.Printf("session closed, exiting")
				return
			}
			pt, err := w.peer.Crypto().Decrypt(buffer[:n])
			w.peer.CryptoRUnlock()
			if err != nil {
				w.logger.Printf("failed to decrypt data: %s", err)
				return
			}
			if spType, spOk := service_packet.TryParseHeader(pt); spOk {
				switch spType {
				case service_packet.RekeyInit:
					if rc := w.peer.RekeyController(); rc != nil {
						w.cp.Handle(pt, w.peer.Egress(), rc)
					}
				case service_packet.Ping:
					w.cp.HandlePing(w.peer.Egress())
				}
				continue
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
