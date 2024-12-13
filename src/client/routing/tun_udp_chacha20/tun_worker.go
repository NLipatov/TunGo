package tun_udp_chacha20

import (
	"context"
	"errors"
	"log"
	"net"
	"time"
	"tungo/crypto/chacha20"
	"tungo/network/ip"
	"tungo/network/keepalive"
)

type udpTunWorker struct {
	router               UDPRouter
	conn                 *net.UDPConn
	session              *chacha20.Session
	sendKeepAliveChan    chan bool
	receiveKeepAliveChan chan bool
	err                  error
}

func newUdpTunWorker() *udpTunWorker {
	return &udpTunWorker{}
}

func (w *udpTunWorker) UseRouter(router UDPRouter) *udpTunWorker {
	if w.err != nil {
		return w
	}

	w.router = router
	return w
}

func (w *udpTunWorker) UseSession(session *chacha20.Session) *udpTunWorker {
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

func (w *udpTunWorker) UseSendKeepAliveChan(ch chan bool) *udpTunWorker {
	if w.err != nil {
		return w
	}

	w.sendKeepAliveChan = ch

	return w
}

func (w *udpTunWorker) UseReceiveKeepAliveChan(ch chan bool) *udpTunWorker {
	if w.err != nil {
		return w
	}

	w.receiveKeepAliveChan = ch

	return w
}

func (w *udpTunWorker) HandlePacketsFromTun(ctx context.Context, triggerReconnect context.CancelFunc) error {
	workerSetupErr := w.err
	if workerSetupErr != nil {
		return workerSetupErr
	}
	buf := make([]byte, ip.MaxPacketLengthBytes)

	// Main loop to read from TUN and send data
	for {
		select {
		case <-ctx.Done(): // Stop-signal
			return nil
		case <-w.sendKeepAliveChan:
			data, err := keepalive.GenerateUDP()
			if err != nil {
				log.Println("failed to generate keep-alive:", err)
				continue
			}
			writeOrReconnect(w.conn, &data, ctx, triggerReconnect)
		default:
			n, err := w.router.tun.Read(buf)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				log.Printf("failed to read from TUN: %v", err)
				triggerReconnect()
			}

			encryptedPacket, nonce, err := w.session.Encrypt(buf[:n])
			if err != nil {
				log.Printf("failed to encrypt packet: %v", err)
				continue
			}

			packet, err := (&chacha20.Packet{}).EncodeUDP(encryptedPacket, nonce)
			if err != nil {
				log.Printf("packet encoding failed: %s", err)
				continue
			}
			writeOrReconnect(w.conn, packet.Payload, ctx, triggerReconnect)
		}
	}
}

func writeOrReconnect(conn *net.UDPConn, data *[]byte, ctx context.Context, connCancel context.CancelFunc) {
	_ = conn.SetWriteDeadline(time.Now().Add(time.Second * 1))
	_, err := conn.Write(*data)
	if err != nil {
		log.Printf("write to UDP failed: %s", err)
		if ctx.Err() != nil {
			return
		}
		connCancel()
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

	for {
		select {
		case <-ctx.Done(): // Stop-signal
			return nil
		default:
			n, _, err := w.conn.ReadFromUDP(buf)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				log.Printf("read from UDP failed: %v", err)
				connCancel()
				return nil
			}

			packet, packetDecodeErr := (&chacha20.Packet{}).DecodeUDP(buf[:n])
			if packetDecodeErr != nil {
				log.Printf("failed to decode a packet: %s", packetDecodeErr)
				continue
			}

			select {
			case w.receiveKeepAliveChan <- true:
				if packet.IsKeepAlive {
					log.Println("keep-alive: OK")
					continue
				}
			default:
			}

			decrypted, _, _, decryptionErr := w.session.DecryptViaNonceBuf(*packet.Payload, packet.Nonce)
			if decryptionErr != nil {
				if errors.Is(decryptionErr, chacha20.ErrNonUniqueNonce) {
					log.Printf("reconnecting on critical decryption err: %s", err)
					connCancel()
					return nil
				}

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
