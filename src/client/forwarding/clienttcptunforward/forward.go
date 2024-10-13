package clienttcptunforward

import (
	"context"
	"encoding/binary"
	"etha-tunnel/handshake/ChaCha20"
	"etha-tunnel/network/keepalive"
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
func ToTCP(conn net.Conn, tunFile *os.File, session *ChaCha20.Session, ctx context.Context, sendKeepAliveChan chan bool) {
	buf := make([]byte, maxPacketLengthBytes)
	tunDataChan := make(chan []byte)

	go func() {
		for {
			n, err := tunFile.Read(buf)
			if err != nil {
				if ctx.Err() != nil {
					fmt.Printf("context ended with error: %s\n", err)
					return
				}
				log.Printf("failed to read from TUN: %v", err)
				continue
			}
			data := make([]byte, n)
			copy(data, buf[:n])
			select {
			case tunDataChan <- data:
			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done(): // Stop-signal
			return
		case <-sendKeepAliveChan:
			keepAliveWriteErr := keepalive.Send(conn)
			if keepAliveWriteErr != nil {
				log.Printf("failed to send keep alive: %s", keepAliveWriteErr)
			}
		case data := <-tunDataChan:
			encryptedPacket, err := session.Encrypt(data)
			if err != nil {
				log.Printf("failed to encrypt packet: %v", err)
				continue
			}

			length := uint32(len(encryptedPacket))
			lengthBuf := make([]byte, 4)
			binary.BigEndian.PutUint32(lengthBuf, length)
			_, err = conn.Write(append(lengthBuf, encryptedPacket...))
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
			length := binary.BigEndian.Uint32(buf[:4])

			if length > maxPacketLengthBytes {
				log.Printf("packet too large: %d", length)
				return
			}

			// Read the encrypted packet based on the length
			_, err = io.ReadFull(conn, buf[:length])
			if err != nil {
				if ctx.Err() != nil {
					// Context was canceled, exit gracefully
					return
				}
				log.Printf("failed to read encrypted packet: %v", err)
				return
			}

			select {
			case receiveKeepAliveChan <- true:
				if length == 9 && string(buf[:length]) == "KEEPALIVE" {
					log.Println("keep-alive: OK")
					continue
				}
			default:
			}

			decrypted, err := session.Decrypt(buf[:length])
			if err != nil {
				log.Printf("failed to decrypt server packet: %v", err)
				return
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
