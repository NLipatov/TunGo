package clienttcptunforward

import (
	"context"
	"encoding/binary"
	"etha-tunnel/handshake/ChaCha20"
	"etha-tunnel/network"
	"etha-tunnel/network/keepalive"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"
)

const (
	maxPacketLengthBytes = 65535
)

// ToTCP forwards packets from TUN to TCP
func ToTCP(conn net.Conn, tunFile *os.File, session *ChaCha20.Session, ctx context.Context, sendKeepAliveChan chan bool) {
	buf := make([]byte, maxPacketLengthBytes)
	var connMutex sync.Mutex

	// Goroutine for handling keepalive signals
	go func() {
		for {
			select {
			case <-ctx.Done(): // Stop-signal
				return
			case <-sendKeepAliveChan:
				connMutex.Lock()
				err := keepalive.Send(conn)
				connMutex.Unlock()
				if err != nil {
					log.Printf("failed to send keep alive: %s", err)
				}
			}
		}
	}()

	// Main loop for reading from TUN and sending to the server
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

			length := uint32(len(encryptedPacket))
			lengthBuf := make([]byte, 4)
			binary.BigEndian.PutUint32(lengthBuf, length)

			connMutex.Lock()
			_, err = conn.Write(append(lengthBuf, encryptedPacket...))
			connMutex.Unlock()
			if err != nil {
				log.Printf("failed to write to server: %v", err)
				return
			}
		}
	}
}

func ToTun(conn net.Conn, tunFile *os.File, session *ChaCha20.Session, ctx context.Context, receiveKeepAliveChan chan bool) {
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
				log.Printf("failed to read from server: %v", err)
				return
			}

			packet, err := (&network.Packet{}).Parse(conn, buf, session)
			if err != nil {
				log.Println(err)
			}

			select {
			case receiveKeepAliveChan <- true:
				if packet.Length == 9 && keepalive.IsKeepAlive(packet.Payload) {
					log.Println("keep-alive: OK")
					continue
				}
			default:
			}

			// Write the decrypted packet to the TUN interface
			_, err = tunFile.Write(packet.Payload)
			if err != nil {
				log.Printf("failed to write to TUN: %v", err)
				return
			}
		}
	}
}
