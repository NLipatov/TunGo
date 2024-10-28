package clienttcptunforward

import (
	"context"
	"etha-tunnel/handshake/ChaCha20"
	"etha-tunnel/network"
	"etha-tunnel/network/keepalive"
	"log"
	"net"
	"os"
)

func TunToUDP(conn *net.UDPConn, tunFile *os.File, session *ChaCha20.Session, ctx context.Context, connCancel context.CancelFunc, sendKeepAliveChan chan bool) {
	buf := make([]byte, maxPacketLengthBytes)
	connWriteChan := make(chan []byte, getConnWriteBufferSize())

	//writes whatever comes from chan to UDP
	go func() {
		for {
			select {
			case <-ctx.Done(): // Stop-signal
				return
			case data := <-connWriteChan:
				_, err := conn.Write(data)
				if err != nil {
					log.Printf("write to UDP failed: %s", err)
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
					log.Printf("context ended with error: %s\n", err)
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

			packet, err := (&network.Packet{}).EncodeUDP(encryptedPacket)
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

func UDPToTun(conn *net.UDPConn, tunFile *os.File, session *ChaCha20.Session, ctx context.Context, connCancel context.CancelFunc, receiveKeepAliveChan chan bool) {
	buf := make([]byte, maxPacketLengthBytes)
	for {
		select {
		case <-ctx.Done(): // Stop-signal
			return
		default:
			n, _, err := conn.ReadFromUDP(buf)
			if err != nil {
				if ctx.Err() != nil {
					log.Printf("context ended with error: %s\n", err)
					return
				}
				log.Printf("read from UDP failed: %v", err)
				connCancel()
				return
			}

			packet, err := (&network.Packet{}).Decode(buf[:n], session)
			if err != nil {
				log.Println(err)
				continue
			}

			if packet.IsKeepAlive {
				log.Println("keep-alive: OK")
				continue
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
