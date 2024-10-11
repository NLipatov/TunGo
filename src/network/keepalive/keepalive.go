package keepalive

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"time"
)

func StartConnectionProbing(connCancel context.CancelFunc, sendKeepAliveChan chan bool, receiveKeepAliveChan chan bool) {
	var sendIntervalSeconds int64 = 15
	var reconnectIntervalSeconds int64 = 45
	lastSent := time.Now().Unix()
	lastReceived := time.Now().Unix()
	go func() {
		for {
			select {
			case r := <-receiveKeepAliveChan:
				if r {
					log.Println("keep-alive: OK")
					lastReceived = time.Now().Unix()
				}
			default:
				if time.Now().Unix()-lastReceived > reconnectIntervalSeconds {
					connCancel()
					return
				}

				if lastSent+sendIntervalSeconds < time.Now().Unix() {
					lastSent = time.Now().Unix()
					sendKeepAliveChan <- true
				}
			}
		}
	}()
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
