package servertcptunforward

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
)

func TunToUDP(tunFile *os.File, localIpMap *sync.Map, localIpToSessionMap *sync.Map, ctx context.Context) {
	log.Fatalf("TunToUDP is not implemented")
}

func UDPToTun(listenPort string, tunFile *os.File, localIpMap *sync.Map, localIpToSessionMap *sync.Map, ctx context.Context) {
	addr, err := net.ResolveUDPAddr("udp", listenPort)
	if err != nil {
		log.Fatalf("failed to resolve udp address: %s", err)
		return
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatalf("failed to listen on port: %s", err)
	}
	defer conn.Close()

	log.Printf("server listening on port %s", listenPort)

	//using this goroutine to 'unblock' Listener.Accept blocking-call
	go func() {
		<-ctx.Done() //blocks till ctx.Done signal comes in
		_ = conn.Close()
		return
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			buffer := make([]byte, IPPacketMaxSizeBytes)
			for {
				_, clientAddr, readFromUdpErr := conn.ReadFromUDP(buffer)
				if readFromUdpErr != nil {
					log.Printf("failed to read from UDP: %s", readFromUdpErr)
					continue
				}
				log.Printf("connected: %s", conn.RemoteAddr())
				session, sessionExists := localIpToSessionMap.Load(clientAddr)
				if !sessionExists {
					//ToDo: implement client UDP registration
					log.Fatalf("udp client registration is not implemented")
				}

				fmt.Println(session)
			}
		}
	}
}

func udpRegisterClient(conn net.Conn, tunFile *os.File, localIpToConn *sync.Map, localIpToServerSessionMap *sync.Map, ctx context.Context) {
	log.Fatalf("udpRegisterClient is not implemented")
}

func udpHandleClient(conn net.Conn, tunFile *os.File, localIpToConn *sync.Map, localIpToSession *sync.Map, extIpAddr *string, ctx context.Context) {
	log.Fatalf("udpHandleClient is not implemented")
}
