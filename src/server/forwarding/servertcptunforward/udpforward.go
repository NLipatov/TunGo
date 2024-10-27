package servertcptunforward

import (
	"context"
	"etha-tunnel/handshake/ChaCha20/handshakeHandlers"
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

	log.Printf("server listening on port udp:%s", listenPort)

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
				session, sessionExists := localIpToSessionMap.Load(clientAddr)
				if !sessionExists {
					//ToDo: implement client UDP registration
					udpRegisterClient(conn, *clientAddr, tunFile, localIpMap, localIpToSessionMap, ctx)
				}

				fmt.Println(session)
			}
		}
	}
}

func udpRegisterClient(conn net.Conn, clientAddr net.UDPAddr, tunFile *os.File, localIpToConn *sync.Map, localIpToServerSessionMap *sync.Map, ctx context.Context) {
	log.Printf("connected: %s", clientAddr.IP.String())

	serverSession, internalIpAddr, err := handshakeHandlers.OnClientConnectedUDP(conn)
	if err != nil {
		_ = conn.Close()
		log.Printf("conn closed: %s (regfail: %s)\n", conn.RemoteAddr(), err)
		return
	}
	log.Printf("registered: %s", clientAddr.IP.String())

	// Prevent IP spoofing
	_, ipCollision := localIpToConn.Load(*internalIpAddr)
	if ipCollision {
		log.Printf("conn closed: %s (internal ip %s already in use)\n", conn.RemoteAddr(), *internalIpAddr)
		_ = conn.Close()
	}

	localIpToServerSessionMap.Store(*internalIpAddr, serverSession)

	handleClient(conn, tunFile, localIpToConn, localIpToServerSessionMap, internalIpAddr, ctx)
}

func udpHandleClient(conn net.Conn, tunFile *os.File, localIpToConn *sync.Map, localIpToSession *sync.Map, extIpAddr *string, ctx context.Context) {
	log.Fatalf("udpHandleClient is not implemented")
}
