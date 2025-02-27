package udp_chacha20

import (
	"context"
	"fmt"
	"log"
	"net"
	"tungo/crypto/chacha20"
	"tungo/network/ip"
	"tungo/network/pipes"
)

type udpTunWorker struct {
	router  *UDPRouter
	conn    *net.UDPConn
	session chacha20.UdpSession
	err     error
	encoder chacha20.UDPEncoder
}

func newUdpTunWorker() *udpTunWorker {
	return &udpTunWorker{}
}

func (w *udpTunWorker) UseRouter(router *UDPRouter) *udpTunWorker {
	if w.err != nil {
		return w
	}

	w.router = router
	return w
}

func (w *udpTunWorker) UseSession(session chacha20.UdpSession) *udpTunWorker {
	if w.err != nil {
		return w
	}

	w.session = session

	return w
}

func (w *udpTunWorker) UseConn(conn *net.UDPConn) *udpTunWorker {
	if w.err != nil {
		return w
	}

	w.conn = conn

	return w
}

func (w *udpTunWorker) UseEncoder(encoder chacha20.UDPEncoder) *udpTunWorker {
	if w.err != nil {
		return w
	}

	w.encoder = encoder

	return w
}

func (w *udpTunWorker) Build() (*udpTunWorker, error) {
	if w.err != nil {
		return nil, w.err
	}

	if w.router == nil {
		return nil, fmt.Errorf("router required but not provided")
	}

	if w.session == nil {
		return nil, fmt.Errorf("session required but not provided")
	}

	if w.conn == nil {
		return nil, fmt.Errorf("connection required but not provided")
	}

	if w.encoder == nil {
		return nil, fmt.Errorf("encoder required but not provided")
	}

	return w, nil
}

func (w *udpTunWorker) HandlePacketsFromTun(ctx context.Context, triggerReconnect context.CancelFunc) error {
	workerSetupErr := w.err
	if workerSetupErr != nil {
		return workerSetupErr
	}
	buf := make([]byte, ip.MaxPacketLengthBytes)

	readerPipe := pipes.
		NewReaderPipe(pipes.
			NewEncryptionPipe(pipes.
				NewDefaultPipe(w.conn), w.session), w.router.tun)

	// Main loop to read from TUN and send data
	for {
		select {
		case <-ctx.Done(): // Stop-signal
			return nil
		default:
			passErr := readerPipe.Pass(buf)
			if passErr != nil {
				if ctx.Err() != nil {
					return nil
				}
				log.Printf("failed to read from TUN: %v", passErr)
				triggerReconnect()
			}
		}
	}
}

func (w *udpTunWorker) HandlePacketsFromConn(ctx context.Context, connCancel context.CancelFunc) error {
	workerSetupErr := w.err
	if workerSetupErr != nil {
		return workerSetupErr
	}
	buf := make([]byte, ip.MaxPacketLengthBytes)

	go func() {
		<-ctx.Done()
		_ = w.conn.Close()
	}()

	pipe := pipes.
		NewReaderPipe(pipes.
			NewDecryptionPipe(pipes.
				NewDefaultPipe(w.router.tun), w.session), w.conn)

	for {
		select {
		case <-ctx.Done(): // Stop-signal
			return nil
		default:
			passErr := pipe.Pass(buf)
			if passErr != nil {
				if ctx.Err() != nil {
					return nil
				}

				log.Printf("read from UDP failed: %v", passErr)
				connCancel()
				return nil
			}
		}
	}
}
