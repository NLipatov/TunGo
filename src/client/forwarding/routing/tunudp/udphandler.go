package tunudp

import (
	"context"
	"log"
	"net"
	"os"
	"time"
	"tungo/handshake/ChaCha20"
	"tungo/network"
	"tungo/network/ip"
	"tungo/network/keepalive"
	"tungo/settings"
)

func FromTun(conn *net.UDPConn, tunFile *os.File, session *ChaCha20.Session, ctx context.Context, connCancel context.CancelFunc, sendKeepAliveChan chan bool) {
	buf := make([]byte, ip.MaxPacketLengthBytes)

	// Main loop to read from TUN and send data
	for {
		select {
		case <-ctx.Done(): // Stop-signal
			return
		case <-sendKeepAliveChan:
			data, err := keepalive.GenerateUDP()
			if err != nil {
				log.Println("failed to generate keep-alive:", err)
				continue
			}
			writeOrReconnect(conn, &data, ctx, connCancel)
		default:
			n, err := tunFile.Read(buf)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("failed to read from TUN: %v", err)
				continue
			}

			encryptedPacket, high, low, err := session.Encrypt(buf[:n])
			if err != nil {
				log.Printf("failed to encrypt packet: %v", err)
				continue
			}

			packet, err := (&network.Packet{}).EncodeUDP(encryptedPacket, &ChaCha20.Nonce{Low: low, High: high})
			if err != nil {
				log.Printf("packet encoding failed: %s", err)
				continue
			}
			writeOrReconnect(conn, packet.Payload, ctx, connCancel)
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

func ToTun(settings settings.ConnectionSettings, conn *net.UDPConn, tunFile *os.File, session *ChaCha20.Session, ctx context.Context, connCancel context.CancelFunc, receiveKeepAliveChan chan bool) {
	buf := make([]byte, ip.MaxPacketLengthBytes)

	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	for {
		select {
		case <-ctx.Done(): // Stop-signal
			return
		default:
			n, _, err := conn.ReadFromUDP(buf)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("read from UDP failed: %v", err)
				connCancel()
				return
			}

			if len(buf[:n]) == len(settings.SessionMarker) && string(buf[:n]) == settings.SessionMarker {
				log.Printf("re-registration request by server")
				connCancel()
				return
			}

			packet, packetDecodeErr := (&network.Packet{}).DecodeUDP(buf[:n])
			if packetDecodeErr != nil {
				log.Printf("failed to decode a packet: %s", packetDecodeErr)
				continue
			}

			select {
			case receiveKeepAliveChan <- true:
				if packet.IsKeepAlive {
					log.Println("keep-alive: OK")
					continue
				}
			default:
			}

			decrypted, _, _, decryptionErr := session.DecryptViaNonceBuf(*packet.Payload, *packet.Nonce)
			if decryptionErr != nil {
				log.Printf("failed to decrypt data: %s", decryptionErr)
				continue
			}

			// Write the decrypted packet to the TUN interface
			_, err = tunFile.Write(decrypted)
			if err != nil {
				log.Printf("failed to write to TUN: %v", err)
				return
			}
		}
	}
}
