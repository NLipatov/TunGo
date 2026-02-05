package udp_chacha20

import (
	"encoding/binary"
	"fmt"
	"io"
	"time"

	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/network/ip"
	"tungo/infrastructure/tunnel/session"
)

// udpDataplaneWorker processes packets for already-established sessions.
//
// It has no socket/TUN read loops; it is a pure "handle one packet" dataplane stage.
type udpDataplaneWorker struct {
	tunWriter io.Writer
	cp        controlPlaneHandler
	now       func() time.Time
}

func newUdpDataplaneWorker(tunWriter io.Writer, cp controlPlaneHandler) *udpDataplaneWorker {
	return &udpDataplaneWorker{
		tunWriter: tunWriter,
		cp:        cp,
		now:       time.Now,
	}
}

func (w *udpDataplaneWorker) HandleEstablished(peer *session.Peer, packet []byte) error {
	// SECURITY: Check closed flag BEFORE using crypto.
	// This prevents use-after-free if peer is being deleted concurrently.
	// The closed flag is set atomically before crypto is zeroed.
	if peer.IsClosed() {
		return nil
	}

	rekeyCtrl := peer.RekeyController()

	decrypted, decryptionErr := peer.Crypto().Decrypt(packet)
	if decryptionErr != nil {
		// Drop: untrusted UDP input can be garbage / attacker-driven.
		return nil
	}

	if rekeyCtrl != nil {
		// Data was successfully decrypted with epoch.
		// Epoch can now be used to encrypt. Allow to encrypt with this epoch by promoting.
		rekeyCtrl.ActivateSendEpoch(binary.BigEndian.Uint16(packet[chacha20.NonceEpochOffset : chacha20.NonceEpochOffset+2]))
		rekeyCtrl.AbortPendingIfExpired(w.now())
		// If service_packet packet - handle it.
		if handled, err := w.cp.Handle(decrypted, peer.Egress(), rekeyCtrl); handled {
			return err
		}
	}

	// Validate source IP against AllowedIPs after decryption
	// Session interface embeds SessionAuth - no type assertion needed
	srcIP, srcOk := ip.ExtractSourceIP(decrypted)
	if !srcOk {
		// Malformed IP header - drop to prevent AllowedIPs bypass
		return nil
	}
	if !peer.Session.IsSourceAllowed(srcIP) {
		// AllowedIPs violation - silently drop for UDP
		return nil
	}

	// Pass it to TUN.
	if _, err := w.tunWriter.Write(decrypted); err != nil {
		return fmt.Errorf("failed to write to TUN: %v", err)
	}
	return nil
}
