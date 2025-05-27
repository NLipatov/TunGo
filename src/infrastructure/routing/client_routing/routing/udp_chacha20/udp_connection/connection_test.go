package udp_connection

import (
	"fmt"
	"net"
	"testing"
	"time"
	"tungo/infrastructure/settings"
)

func TestDefaultConnection_Establish(t *testing.T) {
	port := 3002
	testSettings := settings.Settings{
		ConnectionIP: "127.0.0.1",
		Port:         fmt.Sprintf("%d", port),
	}

	serverAcceptChan := make(chan struct{}, 1)
	serverErrChan := make(chan error, 1)

	addr := net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: port,
	}

	listener, err := net.ListenUDP("udp", &addr)
	if err != nil {
		t.Fatal(err)
	}

	defer func(listener *net.UDPConn) {
		_ = listener.Close()
	}(listener)

	go func() {

		for {
			buffer := make([]byte, 1)
			_, _, acceptErr := listener.ReadFromUDP(buffer)
			if acceptErr != nil {
				serverErrChan <- acceptErr
				return
			}
			serverAcceptChan <- struct{}{}
			return
		}
	}()

	connection := NewConnection(testSettings)
	conn, connErr := connection.Establish()
	if connErr != nil {
		t.Fatal(connErr)
	}
	_, writeErr := conn.Write([]byte{1})
	if writeErr != nil {
		t.Fatal(writeErr)
	}

	select {
	case <-time.After(time.Duration(1) * time.Second):
		t.Fatalf("timeout")
	case serverErr := <-serverErrChan:
		t.Fatalf("server error: %s", serverErr)
	case <-serverAcceptChan:
		return
	}
}
