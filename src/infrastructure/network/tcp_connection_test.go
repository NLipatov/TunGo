package network

import (
	"fmt"
	"net"
	"testing"
	"time"
	"tungo/infrastructure/settings"
)

func TestDefaultConnection_Establish(t *testing.T) {
	port := 3001
	testSettings := settings.Settings{
		ConnectionIP: "127.0.0.1",
		Port:         fmt.Sprintf("%d", port),
	}

	serverAcceptChan := make(chan struct{}, 1)
	serverErrChan := make(chan error, 1)

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		t.Fatal(err)
	}

	defer func(listener net.Listener) {
		_ = listener.Close()
	}(listener)

	go func() {

		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				serverErrChan <- acceptErr
				return
			}
			_ = conn.Close()
			serverAcceptChan <- struct{}{}
			return
		}
	}()

	socket, socketErr := NewSocket(testSettings.ConnectionIP, testSettings.Port)
	if socketErr != nil {
		t.Fatal(socketErr)
	}
	connection := NewTcpConnection(socket)
	_, connErr := connection.Establish()
	if connErr != nil {
		t.Fatal(connErr)
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
