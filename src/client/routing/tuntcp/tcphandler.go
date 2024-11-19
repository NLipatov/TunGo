package tuntcp

import (
	"context"
	"encoding/binary"
	"io"
	"log"
	"net"
	"os"
	"tungo/handshake/ChaCha20"
	"tungo/network"
	"tungo/network/keepalive"
	"tungo/settings/client"
)

// ToTCP forwards packets from TUN to TCP
func ToTCP(conn net.Conn, tunFile *os.File, session *ChaCha20.Session, ctx context.Context, connCancel context.CancelFunc, sendKeepaliveCh chan bool) {
	buf := make([]byte, network.IPPacketMaxSizeBytes)
	connWriteChan := make(chan []byte, getConnWriteBufferSize())

	go func() {
		<-ctx.Done()
		_ = conn.Close()
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
				_, err := conn.Write(data)
				if err != nil {
					log.Printf("write to TCP failed: %s", err)
					connCancel()
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
			case <-sendKeepaliveCh:
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
			return
		default:
			n, err := tunFile.Read(buf)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("failed to read from TUN: %v", err)
				connCancel()
			}

			encryptedPacket, _, _, err := session.Encrypt(buf[:n])
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
				return
			case connWriteChan <- packet.Payload:
			}
		}
	}
}

func ToTun(conn net.Conn, tunFile *os.File, session *ChaCha20.Session, ctx context.Context, connCancel context.CancelFunc, receiveKeepaliveCh chan bool) {
	buf := make([]byte, network.IPPacketMaxSizeBytes)

	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	for {
		select {
		case <-ctx.Done(): // Stop-signal
			return
		default:
			_, err := io.ReadFull(conn, buf[:4])
			if err != nil {
				if ctx.Err() != nil {
					return
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
			_, err = io.ReadFull(conn, buf[:length])
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
			case receiveKeepaliveCh <- true:
				//shortcut for keep alive response case
				if packet.IsKeepAlive {
					log.Println("keep-alive: OK")
					continue
				}
			default:
			}

			decrypted, _, _, decryptionErr := session.Decrypt(packet.Payload)
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

func getConnWriteBufferSize() int32 {
	conf, err := (&client.Conf{}).Read()
	if err != nil {
		log.Println("failed to read connection buffer size from client configuration. Using fallback value: 125 000")
		return 125_000
	}

	return conf.TCPWriteChannelBufferSize
}
