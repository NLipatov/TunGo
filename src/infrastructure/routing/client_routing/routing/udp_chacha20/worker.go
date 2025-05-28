package udp_chacha20

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"tungo/application"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/network"
)

type UdpWorker struct {
	tunHandler          application.TunHandler
	ctx                 context.Context
	adapter             application.ConnectionAdapter
	tun                 io.ReadWriteCloser
	cryptographyService application.CryptographyService
}

func NewUdpWorker(
	TunHandler application.TunHandler,
	ctx context.Context,
	adapter application.ConnectionAdapter,
	tun io.ReadWriteCloser,
	cryptographyService application.CryptographyService,
) *UdpWorker {
	return &UdpWorker{
		tunHandler:          TunHandler,
		ctx:                 ctx,
		adapter:             adapter,
		tun:                 tun,
		cryptographyService: cryptographyService,
	}
}

func (w *UdpWorker) HandleTun() error {
	return w.tunHandler.HandleTun()
}

func (w *UdpWorker) HandleTransport() error {
	dataBuf := make([]byte, network.MaxPacketLengthBytes+12)

	for {
		select {
		case <-w.ctx.Done():
			return nil
		default:
			n, readErr := w.adapter.Read(dataBuf)
			if readErr != nil {
				if errors.Is(readErr, os.ErrDeadlineExceeded) {
					continue
				}

				if w.ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("could not read a packet from adapter: %v", readErr)
			}

			if n == 1 && network.SignalIs(dataBuf[0], network.SessionReset) {
				return fmt.Errorf("server requested cryptographyService reset")
			}

			decrypted, decryptionErr := w.cryptographyService.Decrypt(dataBuf[:n])
			if decryptionErr != nil {
				if w.ctx.Err() != nil {
					return nil
				}

				// Duplicate nonce detected – this may indicate a network retransmission or a replay attack.
				// In either case, skip this packet.
				if errors.Is(decryptionErr, chacha20.ErrNonUniqueNonce) {
					continue
				}
				return fmt.Errorf("failed to decrypt data: %s", decryptionErr)
			}

			_, writeErr := w.tun.Write(decrypted)
			if writeErr != nil {
				if w.ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("failed to write to TUN: %s", writeErr)
			}
		}
	}
}
