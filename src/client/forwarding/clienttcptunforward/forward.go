package clienttcptunforward

import (
	"context"
	"encoding/binary"
	"etha-tunnel/handshake/ChaCha20"
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

var S2CMutex sync.Mutex
var C2SMutex sync.Mutex

// ToTCP forwards packets from TUN to TCP
func ToTCP(conn net.Conn, tunFile *os.File, session ChaCha20.Session, ctx context.Context) {
	buf := make([]byte, maxPacketLengthBytes)
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
			}

			aad := session.CreateAAD(false, session.SendNonce)

			encryptedPacket, err := session.Encrypt(buf[:n], aad)
			if err != nil {
				log.Printf("failed to encrypt packet: %v", err)
			}

			length := uint32(len(encryptedPacket))
			lengthBuf := make([]byte, 4)
			binary.BigEndian.PutUint32(lengthBuf, length)
			_, err = conn.Write(append(lengthBuf, encryptedPacket...))
			if err != nil {
				log.Printf("failed to write to server: %v", err)
			}
		}
	}
}

func ToTun(conn net.Conn, tunFile *os.File, session ChaCha20.Session, ctx context.Context) {
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
			}
			length := binary.BigEndian.Uint32(buf[:4])

			if length > maxPacketLengthBytes {
				log.Printf("packet too large: %d", length)
			}

			// Read the encrypted packet based on the length
			_, err = io.ReadFull(conn, buf[:length])
			if err != nil {
				if ctx.Err() != nil {
					// Context was canceled, exit gracefully
					return
				}
				log.Printf("failed to read encrypted packet: %v", err)
			}

			aad := session.CreateAAD(true, session.RecvNonce)
			decrypted, err := session.Decrypt(buf[:length], aad)
			if err != nil {
				log.Printf("failed to decrypt server packet: %v", err)
			}

			// Write the decrypted packet to the TUN interface
			_, err = tunFile.Write(decrypted)
			if err != nil {
				log.Printf("failed to write to TUN: %v", err)
			}
		}
	}
}
