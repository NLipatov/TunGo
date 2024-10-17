package keepalive

import (
	"bytes"
	"context"
	"encoding/binary"
	"time"
)

var keepAlivePacketContent = [9]byte{'K', 'E', 'E', 'P', 'A', 'L', 'I', 'V', 'E'}

func StartConnectionProbing(ctx context.Context, connCancel context.CancelFunc, sendKeepAliveChan chan bool, receiveKeepAliveChan chan bool) {
	sendInterval := time.Duration(25) * time.Second
	reconnectInterval := time.Duration(35) * time.Second
	lastPacket := time.Now()

	ticker := time.NewTicker(sendInterval - sendInterval/4)
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

				if time.Since(lastPacket) < sendInterval {
					continue
				}

				sendKeepAliveChan <- true
			}
		}
	}()
	<-ctx.Done()
}

func Generate() ([]byte, error) {
	lengthBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuf, uint32(len(keepAlivePacketContent)))
	return append(lengthBuf, keepAlivePacketContent[:]...), nil
}

func IsKeepAlive(data []byte) bool {
	if bytes.Equal(data, keepAlivePacketContent[:]) {
		return true
	}
	return false
}
