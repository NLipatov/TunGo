package tcptunforward

import (
	"context"
	"encoding/binary"
	"etha-tunnel/handshake/ChaCha20"
	"fmt"
	"io"
	"net"
	"os"
	"sync/atomic"
)

const (
	maxPacketLengthBytes = 65535
)

// ToTCP forwards packets from TUN to TCP
func ToTCP(conn net.Conn, tunFile *os.File, session ChaCha20.Session, ctx context.Context) error {
	buf := make([]byte, maxPacketLengthBytes)
	for {
		select {
		case <-ctx.Done(): // Stop-signal
			return nil
		default:
			n, err := tunFile.Read(buf)
			if err != nil {
				if ctx.Err() != nil {
					fmt.Printf("context ended with error: %s\n", err)
					return nil
				}
				return fmt.Errorf("failed to read from TUN: %v", err)
			}

			aad := session.CreateAAD(false, session.C2SCounter)

			encryptedPacket, err := session.Encrypt(buf[:n], aad)
			if err != nil {
				return fmt.Errorf("failed to encrypt packet: %v", err)
			}

			atomic.AddUint64(&session.C2SCounter, 1)

			length := uint32(len(encryptedPacket))
			lengthBuf := make([]byte, 4)
			binary.BigEndian.PutUint32(lengthBuf, length)
			_, err = conn.Write(append(lengthBuf, encryptedPacket...))
			if err != nil {
				return fmt.Errorf("failed to write to server: %v", err)
			}
		}
	}
}

func ToTun(conn net.Conn, tunFile *os.File, session ChaCha20.Session, ctx context.Context) error {
	buf := make([]byte, maxPacketLengthBytes)
	for {
		select {
		case <-ctx.Done(): // Stop-signal
			return nil
		default:
			_, err := io.ReadFull(conn, buf[:4])
			if err != nil {
				if ctx.Err() != nil {
					fmt.Printf("context ended with error: %s\n", err)
					return nil
				}
				return fmt.Errorf("failed to read from server: %v", err)
			}
			length := binary.BigEndian.Uint32(buf[:4])

			if length > maxPacketLengthBytes {
				return fmt.Errorf("packet too large: %d", length)
			}

			// Read the encrypted packet based on the length
			_, err = io.ReadFull(conn, buf[:length])
			if err != nil {
				if ctx.Err() != nil {
					// Context was canceled, exit gracefully
					return nil
				}
				return fmt.Errorf("failed to read encrypted packet: %v", err)
			}

			aad := session.CreateAAD(true, session.S2CCounter)
			decrypted, err := session.Decrypt(buf[:length], aad)
			if err != nil {
				return fmt.Errorf("failed to decrypt server packet: %v", err)
			}

			atomic.AddUint64(&session.S2CCounter, 1)

			// Write the decrypted packet to the TUN interface
			_, err = tunFile.Write(decrypted)
			if err != nil {
				return fmt.Errorf("failed to write to TUN: %v", err)
			}
		}
	}
}