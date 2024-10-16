package keepalive

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"time"
)

var keepAlivePacketContent = []byte{'K', 'E', 'E', 'P', 'A', 'L', 'I', 'V', 'E'}

func StartConnectionProbing(ctx context.Context, connCancel context.CancelFunc, sendKeepAliveChan chan bool, receiveKeepAliveChan chan bool) {
	sendInterval := time.Duration(25) * time.Second
	reconnectInterval := time.Duration(35) * time.Second
	lastPacket := time.Now()

	ticker := time.NewTicker(sendInterval)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case r := <-receiveKeepAliveChan:
				if r {
					lastPacket = time.Now()
				}
			case <-ticker.C:
				if time.Since(lastPacket) > reconnectInterval {
					connCancel()
					return
				}

				sendKeepAliveChan <- true
			}
		}
	}()
	<-ctx.Done()
}

func Send(conn net.Conn) error {
	keepAliveMessage := []byte("KEEPALIVE")
	lengthBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuf, uint32(len(keepAliveMessage)))
	_, keepAliveWriteErr := conn.Write(append(lengthBuf, keepAliveMessage...))
	if keepAliveWriteErr != nil {
		return fmt.Errorf("failed to send keep alive: %s", keepAliveWriteErr)
	}

	return nil
}

func IsKeepAlive(data []byte) bool {
	if bytes.Equal(data, keepAlivePacketContent) {
		return true
	}
	return false
}
