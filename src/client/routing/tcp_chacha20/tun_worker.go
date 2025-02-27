package tcp_chacha20

import (
	"context"
	"fmt"
	"log"
	"net"
	"tungo/crypto"
	"tungo/crypto/chacha20"
	"tungo/network"
	"tungo/network/pipes"
)

type tcpTunWorker struct {
	router  *TCPRouter
	conn    net.Conn
	session crypto.Session
	encoder chacha20.TCPEncoder
	err     error
}

func newTcpTunWorker() *tcpTunWorker {
	return &tcpTunWorker{}
}

func (w *tcpTunWorker) UseRouter(router *TCPRouter) *tcpTunWorker {
	if w.err != nil {
		return w
	}

	w.router = router
	return w
}

func (w *tcpTunWorker) UseSession(session crypto.Session) *tcpTunWorker {
	if w.err != nil {
		return w
	}

	w.session = session

	return w
}

func (w *tcpTunWorker) UseConn(conn net.Conn) *tcpTunWorker {
	if w.err != nil {
		return w
	}

	w.conn = conn

	return w
}

func (w *tcpTunWorker) UseEncoder(encoder *chacha20.DefaultTCPEncoder) *tcpTunWorker {
	if w.err != nil {
		return w
	}

	w.encoder = encoder

	return w
}

func (w *tcpTunWorker) Build() (*tcpTunWorker, error) {
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

func (w *tcpTunWorker) HandlePacketsFromTun(ctx context.Context, triggerReconnect context.CancelFunc) error {
	workerSetupErr := w.err
	if workerSetupErr != nil {
		return workerSetupErr
	}
	buf := make([]byte, network.IPPacketMaxSizeBytes)

	go func() {
		<-ctx.Done()
		_ = w.conn.Close()
	}()

	readerPipe := pipes.
		NewReaderPipe(pipes.
			NewEncryptionPipe(pipes.
				NewTcpEncodingPipe(pipes.
					NewDefaultPipe(w.conn), w.encoder), w.session), w.router.tun)

	//passes anything from tun to chan
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
				log.Printf("write to TCP failed: %v", passErr)
				triggerReconnect()
			}
		}
	}
}

func (w *tcpTunWorker) HandlePacketsFromConn(ctx context.Context, connCancel context.CancelFunc) error {
	workerSetupErr := w.err
	if workerSetupErr != nil {
		return workerSetupErr
	}
	buf := make([]byte, network.IPPacketMaxSizeBytes)

	go func() {
		<-ctx.Done()
		_ = w.conn.Close()
	}()

	pipe := pipes.
		NewTCPReaderPipe(pipes.
			NewTCPDecodingPipe(pipes.
				NewDecryptionPipe(pipes.
					NewDefaultPipe(w.router.tun), w.session), w.encoder), w.conn, w.router.tun)

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
				log.Printf("read from TCP failed: %v", passErr)
				connCancel()
			}
		}
	}
}
