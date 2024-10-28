package clienttcptunforward

import (
	"context"
	"encoding/binary"
	"etha-tunnel/handshake/ChaCha20"
	"etha-tunnel/network"
	"etha-tunnel/network/keepalive"
	"etha-tunnel/server/forwarding/servertcptunforward"
	"etha-tunnel/settings/client"
	"fmt"
	"io"
	"log"
	"net"
	"os"
)

const (
	maxPacketLengthBytes = 65535
)

// ToTCP forwards packets from TUN to TCP
func ToTCP(conn net.Conn, tunFile *os.File, session *ChaCha20.Session, ctx context.Context, connCancel context.CancelFunc, sendKeepAliveChan chan bool) {
	buf := make([]byte, maxPacketLengthBytes)
	connWriteChan := make(chan []byte, getConnWriteBufferSize())

	//writes whatever comes from chan to TCP
	go func() {
		for {
			select {
			case <-ctx.Done(): // Stop-signal
				return
			case data := <-connWriteChan:
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
			case <-sendKeepAliveChan:
				data, err := keepalive.Generate()
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
					fmt.Printf("context ended with error: %s\n", err)
					return
				}
				log.Printf("failed to read from TUN: %v", err)
				continue
			}

			encryptedPacket, err := session.Encrypt(buf[:n])
			if err != nil {
				log.Printf("failed to encrypt packet: %v", err)
				continue
			}

			packet, err := (&network.Packet{}).Encode(encryptedPacket)
			if err != nil {
				log.Printf("packet encoding failed: %s", err)
				continue
			}

			select {
			case connWriteChan <- packet.Payload:
			case <-ctx.Done():
				return
			}
		}
	}
}

func ToTun(conn net.Conn, tunFile *os.File, session *ChaCha20.Session, ctx context.Context, connCancel context.CancelFunc, receiveKeepAliveChan chan bool) {
	buf := make([]byte, maxPacketLengthBytes)
	for {
		select {
		case <-ctx.Done(): // Stop-signal
			return
		default:
			_, err := io.ReadFull(conn, buf[:4])
			if err != nil {
				if ctx.Err() != nil {
					fmt.Printf("context ended with error: %s\n", err)
					return
				}
				log.Printf("read from TCP failed: %v", err)
				connCancel()
				return
			}

			//read packet length from 4-byte length prefix
			var length = binary.BigEndian.Uint32(buf[:4])
			if length < 4 || length > servertcptunforward.IPPacketMaxSizeBytes {
				log.Printf("invalid packet Length: %d", length)
				continue
			}

			//read n-bytes from connection
			_, err = io.ReadFull(conn, buf[:length])
			if err != nil {
				log.Printf("failed to read packet from connection: %s", err)
				continue
			}

			packet, err := (&network.Packet{}).Decode(buf[:length])
			if err != nil {
				log.Println(err)
			}

			select {
			//refreshes last packet time
			case receiveKeepAliveChan <- true:
				//shortcut for keep alive response case
				if packet.Length == 9 && keepalive.IsKeepAlive(packet.Payload) {
					log.Println("keep-alive: OK")
					continue
				}
			default:
			}

			decrypted, decryptionErr := session.Decrypt(packet.Payload)
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
		log.Println("failed to read connection buffer size from client configuration. Using fallback value: 1000")
		return 1000
	}

	return conf.TCPWriteChannelBufferSize
}
