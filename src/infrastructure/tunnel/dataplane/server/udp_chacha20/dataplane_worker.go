package udp_chacha20

import (
	"encoding/binary"
	"fmt"
	"io"

	chacha20 "tungo/infrastructure/cryptography/chacha20/udp"
	"tungo/infrastructure/network/ip"
	"tungo/infrastructure/telemetry/trafficstats"
	"tungo/infrastructure/tunnel/session"
)

// udpDataplaneWorker processes packets for already-established sessions.
//
// It has no socket/TUN read loops; it is a pure "handle one packet" dataplane stage.
type udpDataplaneWorker struct {
	tunWriter io.Writer
	cp        controlPlaneHandler
}

func newUdpDataplaneWorker(tunWriter io.Writer, cp controlPlaneHandler) *udpDataplaneWorker {
	return &udpDataplaneWorker{
		tunWriter: tunWriter,
		cp:        cp,
	}
}

func (w *udpDataplaneWorker) HandleEstablished(peer *session.Peer, packet []byte) error {
	// Use tryDecryptSafe to guard against TOCTOU race with idle reaper:
	// between IsClosed() check and Decrypt(), the reaper may Zeroize crypto.
	decrypted, ok := tryDecryptSafe(peer, packet)
	if !ok {
		return nil
	}

	return w.handleDecrypted(peer, packet, decrypted)
}

// handleDecrypted processes a successfully decrypted packet for the given peer.
// Separated from HandleEstablished so the roaming path can reuse it without
// double-decrypting.
func (w *udpDataplaneWorker) handleDecrypted(peer *session.Peer, rawPacket, decrypted []byte) error {
	// Record activity AFTER successful decryption so attackers cannot
	// keep a session alive by sending garbage to its external address.
	peer.TouchActivity()

	rekeyCtrl := peer.RekeyController()
	if rekeyCtrl != nil {
		// Authentication proves the peer epoch; UDP uses the same event to promote send.
		epoch := binary.BigEndian.Uint16(rawPacket[chacha20.EpochOffset : chacha20.EpochOffset+2])
		rekeyCtrl.ObservePeerEpoch(epoch)
		rekeyCtrl.ActivateSendEpoch(epoch)
		// If service_packet packet - handle it.
		// Note: On EpochExhausted, server sends EpochExhausted packet to client.
		// Session stays alive until client reconnects with fresh handshake.
		if coordinator, ok := rekeyCtrl.(rekeyController); ok {
			if handled, _ := w.cp.Handle(epoch, decrypted, peer.Egress(), coordinator); handled {
				return nil
			}
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
	trafficstats.AddRX(len(decrypted))
	return nil
}
