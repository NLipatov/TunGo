package tcp_chacha20

import (
	"context"
	"io"
	"log"
	"tungo/application"
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
	cryptographyService application.CryptographyService) application.TunHandler {
	return &TunHandler{
		ctx:                 ctx,
		reader:              reader,
		writer:              writer,
		cryptographyService: cryptographyService,
	}
}

func (t *TunHandler) HandleTun() error {
	backing := make([]byte, settings.DefaultEthernetMTU+settings.TCPChacha20Overhead)
	pt := backing[:settings.DefaultEthernetMTU] // len=SafeMTU, cap=SafeMTU+16

	//passes anything from tun to chan
	for {
		select {
		case <-t.ctx.Done():
			return nil
		default:
			n, err := t.reader.Read(pt)
			if err != nil {
				if t.ctx.Err() != nil {
					return nil
				}
				log.Printf("failed to read from TUN: %v", err)
				return err
			}

			ct, encryptErr := t.cryptographyService.Encrypt(pt[:n])
			if encryptErr != nil {
				log.Printf("failed to encrypt packet: %v", encryptErr)
				return encryptErr
			}

			_, err = t.writer.Write(ct)
			if err != nil {
				log.Printf("write to TCP failed: %s", err)
				return err
			}
		}
	}
}
