package tcp_chacha20

import (
	"context"
	"io"
	"log"
	"tungo/application"
	"tungo/application/network/tun"
	"tungo/infrastructure/settings"
)

type TunHandler struct {
	ctx                 context.Context
	reader              io.Reader // abstraction over TUN device
	writer              io.Writer // abstraction over transport
	cryptographyService application.CryptographyService
}

func NewTunHandler(ctx context.Context,
	reader io.Reader,
	writer io.Writer,
	cryptographyService application.CryptographyService) tun.Handler {
	return &TunHandler{
		ctx:                 ctx,
		reader:              reader,
		writer:              writer,
		cryptographyService: cryptographyService,
	}
}

func (t *TunHandler) HandleTun() error {
	// buffer has settings.TCPChacha20Overhead headroom for in-place encryption
	// payload itself will take settings.DefaultEthernetMTU bytes
	var buffer [settings.DefaultEthernetMTU + settings.TCPChacha20Overhead]byte
	payload := buffer[:settings.DefaultEthernetMTU]

	//passes anything from tun to chan
	for {
		select {
		case <-t.ctx.Done():
			return nil
		default:
			n, err := t.reader.Read(payload)
			if err != nil {
				if t.ctx.Err() != nil {
					return nil
				}
				log.Printf("failed to read from TUN: %v", err)
				return err
			}

			ciphertext, ciphertextErr := t.cryptographyService.Encrypt(payload[:n])
			if ciphertextErr != nil {
				log.Printf("failed to encrypt packet: %v", ciphertextErr)
				return ciphertextErr
			}

			_, err = t.writer.Write(ciphertext)
			if err != nil {
				log.Printf("write to TCP failed: %s", err)
				return err
			}
		}
	}
}
