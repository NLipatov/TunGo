package tun_tcp

import (
	"context"
	"encoding/binary"
	"io"
	"log"
	"net"
	"tungo/handshake/chacha20"
	"tungo/network"
	"tungo/network/keepalive"
	"tungo/settings/client"
)

type tcpTunWorker struct {
	router               TCPRouter
	conn                 net.Conn
	session              *chacha20.Session
	sendKeepAliveChan    chan bool
	receiveKeepAliveChan chan bool
	err                  error
}

func newTcpTunWorker() *tcpTunWorker {
	return &tcpTunWorker{}
}

func (w *tcpTunWorker) UseRouter(router TCPRouter) *tcpTunWorker {
	if w.err != nil {
		return w
	}

	w.router = router
	return w
}

func (w *tcpTunWorker) UseSession(session *chacha20.Session) *tcpTunWorker {
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

func (w *tcpTunWorker) UseSendKeepAliveChan(ch chan bool) *tcpTunWorker {
	if w.err != nil {
		return w
	}

	w.sendKeepAliveChan = ch

	return w
}

func (w *tcpTunWorker) UseReceiveKeepAliveChan(ch chan bool) *tcpTunWorker {
	if w.err != nil {
		return w
	}

	w.receiveKeepAliveChan = ch

	return w
}

func (w *tcpTunWorker) HandlePacketsFromTun(ctx context.Context, triggerReconnect context.CancelFunc) error {
	workerSetupErr := w.err
	if workerSetupErr != nil {
		return workerSetupErr
	}
	buf := make([]byte, network.IPPacketMaxSizeBytes)
	connWriteChan := make(chan []byte, getConnWriteBufferSize())

	go func() {
		<-ctx.Done()
		_ = w.conn.Close()
	}()

	//writes whatever comes from chan to TCP
	go func() {
		for {
			select {
			case <-ctx.Done(): // Stop-signal
				return
			case data, ok := <-connWriteChan:
				if !ok { //if connWriteChan is closed
					return
				}
				_, err := w.conn.Write(data)
				if err != nil {
					log.Printf("write to TCP failed: %s", err)
					triggerReconnect()
					return
				}
			}
		}
	}()

	//passes keepalive messages to chan
	go func() {
		for {
			select {
			case <-ctx.Done(): // Stop-signal
				return
			case <-w.sendKeepAliveChan:
				data, err := keepalive.GenerateTCP()
				if err != nil {
					log.Println(err)
				}
				select {
				case connWriteChan <- data:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	//passes anything from tun to chan
	for {
		select {
		case <-ctx.Done(): // Stop-signal
			return nil
		default:
			n, err := w.router.tun.Read(buf)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				log.Printf("failed to read from TUN: %v", err)
				triggerReconnect()
			}

			encryptedPacket, _, _, err := w.session.Encrypt(buf[:n])
			if err != nil {
				log.Printf("failed to encrypt packet: %v", err)
				continue
			}

			packet, err := (&network.Packet{}).EncodeTCP(encryptedPacket)
			if err != nil {
				log.Printf("packet encoding failed: %s", err)
				continue
			}

			select {
			case <-ctx.Done():
				close(connWriteChan)
				return nil
			case connWriteChan <- packet.Payload:
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

	for {
		select {
		case <-ctx.Done(): // Stop-signal
			return nil
		default:
			_, err := io.ReadFull(w.conn, buf[:4])
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				log.Printf("read from TCP failed: %v", err)
				connCancel()
			}

			//read packet length from 4-byte length prefix
			var length = binary.BigEndian.Uint32(buf[:4])
			if length < 4 || length > network.IPPacketMaxSizeBytes {
				log.Printf("invalid packet Length: %d", length)
				continue
			}

			//read n-bytes from connection
			_, err = io.ReadFull(w.conn, buf[:length])
			if err != nil {
				log.Printf("failed to read packet from connection: %s", err)
				continue
			}

			packet, err := (&network.Packet{}).DecodeTCP(buf[:length])
			if err != nil {
				log.Println(err)
			}

			select {
			//refreshes last packet time
			case w.receiveKeepAliveChan <- true:
				//shortcut for keep alive response case
				if packet.IsKeepAlive {
					log.Println("keep-alive: OK")
					continue
				}
			default:
			}

			decrypted, _, _, decryptionErr := w.session.Decrypt(packet.Payload)
			if decryptionErr != nil {
				log.Printf("failed to decrypt data: %s", decryptionErr)
				continue
			}

			// Write the decrypted packet to the TUN interface
			_, err = w.router.tun.Write(decrypted)
			if err != nil {
				log.Printf("failed to write to TUN: %v", err)
				return err
			}
		}
	}
}

func getConnWriteBufferSize() int32 {
	conf, err := (&client.Conf{}).Read()
	if err != nil {
		log.Println("failed to read connection buffer size from client configuration. Using fallback value: 125 000")
		return 125_000
	}

	return conf.TCPWriteChannelBufferSize
}
