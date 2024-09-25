package network

import (
	"encoding/binary"
	"etha-tunnel/handshake/ChaCha20"
	"etha-tunnel/handshake/ChaCha20/handshakeHandlers"
	"etha-tunnel/network/packets"
	"etha-tunnel/network/utils"
	"etha-tunnel/settings/server"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"
)

func ServeConnections(tunFile *os.File, listenPort string) error {
	err := createNewTun()
	err = configureServer(tunFile)
	if err != nil {
		return fmt.Errorf("failed to configure a server: %s\n", err)
	}
	defer undoConfigureSever(tunFile)

	// Map to keep track of connected clients
	var extIpToLocalIp sync.Map
	var extIpToCryptoSession sync.Map

	var wg sync.WaitGroup
	wg.Add(2)

	// TUN -> TCP
	go func() {
		defer wg.Done()
		forwardTunToTCP(tunFile, &extIpToLocalIp, &extIpToCryptoSession)
	}()

	// TCP -> TUN
	go func() {
		defer wg.Done()
		forwardTCPToTun(listenPort, tunFile, &extIpToLocalIp, &extIpToCryptoSession)
	}()

	wg.Wait()
	return nil
}

func configureServer(tunFile *os.File) error {
	externalIfName, err := utils.GetDefaultIf()
	if err != nil {
		return err
	}

	err = EnableNAT(externalIfName)
	if err != nil {
		return fmt.Errorf("failed enabling NAT: %v", err)
	}

	err = setupForwarding(tunFile, externalIfName)
	if err != nil {
		return fmt.Errorf("failed to set up forwarding: %v", err)
	}

	return err
}

func undoConfigureSever(tunFile *os.File) {
	tunName, err := getIfName(tunFile)
	if err != nil {
		log.Printf("failed to determing tunnel ifName: %s\n", err)
	}

	err = DisableNAT(tunName)
	if err != nil {
		log.Printf("failed to disbale NAT: %s\n", err)
	}

	err = clearForwarding(tunFile, tunName)
	if err != nil {
		log.Printf("failed to disbale forwarding: %s\n", err)
	}
}

func forwardTunToTCP(tunFile *os.File, localIpMap *sync.Map, localIpToSessionMap *sync.Map) {
	buf := make([]byte, 65535)
	for {
		n, err := tunFile.Read(buf)
		if err != nil {
			log.Printf("failed to read from TUN: %v", err)
			continue
		}
		packet := buf[:n]
		if len(packet) < 1 {
			log.Printf("invalid IP packet")
			continue
		}

		header, err := packets.Parse(packet)
		if err != nil {
			log.Printf("failed to parse a IPv4 header")
			continue
		}
		destinationIP := header.GetDestinationIP().String()
		v, ok := localIpMap.Load(destinationIP)
		if ok {
			conn := v.(net.Conn)
			///
			sessionValue, sessionExists := localIpToSessionMap.Load(destinationIP)
			if !sessionExists {
				log.Printf("failed to load session")
				continue
			}
			session := sessionValue.(*ChaCha20.Session)
			aad := session.CreateAAD(true, session.S2CCounter)
			encryptedPacket, err := session.Encrypt(packet, aad)
			if err != nil {
				log.Printf("failder to encrypt a package")
				continue
			}
			atomic.AddUint64(&session.S2CCounter, 1)

			length := uint32(len(encryptedPacket))
			lengthBuf := make([]byte, 4)
			binary.BigEndian.PutUint32(lengthBuf, length)
			_, err = conn.Write(append(lengthBuf, encryptedPacket...))
			if err != nil {
				log.Printf("failed to send packet to client: %v", err)
				localIpMap.Delete(destinationIP)
				localIpToSessionMap.Delete(destinationIP)
			}
		}
	}
}

func forwardTCPToTun(listenPort string, tunFile *os.File, localIpMap *sync.Map, localIpToSessionMap *sync.Map) {
	listener, err := net.Listen("tcp", listenPort)
	if err != nil {
		log.Printf("failed to listen on port %s: %v", listenPort, err)
	}
	defer listener.Close()
	log.Printf("server listening on port %s", listenPort)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("failed to accept connection: %v", err)
			continue
		}
		go registerClient(conn, tunFile, localIpMap, localIpToSessionMap)
	}
}

func registerClient(conn net.Conn, tunFile *os.File, localIpToConn *sync.Map, localIpToServerSessionMap *sync.Map) {
	log.Printf("client connected: %s", conn.RemoteAddr())

	serverSession, extIpAddr, err := handshakeHandlers.OnClientConnected(conn)
	if err != nil {
		log.Printf("failed register a client: %s\n", err)
	}

	localIpToConn.Store(*extIpAddr, conn)
	localIpToServerSessionMap.Store(*extIpAddr, serverSession)

	handleClient(conn, tunFile, localIpToConn, localIpToServerSessionMap, extIpAddr)
}

func handleClient(conn net.Conn, tunFile *os.File, localIpToConn *sync.Map, localIpToSession *sync.Map, extIpAddr *string) {
	defer func() {
		localIpToConn.Delete(*extIpAddr)
		localIpToSession.Delete(*extIpAddr)
		conn.Close()
		log.Printf("client disconnected: %s", conn.RemoteAddr())
	}()

	buf := make([]byte, 65535)
	for {
		// Read the length of the encrypted packet (4 bytes)
		_, err := io.ReadFull(conn, buf[:4])
		if err != nil {
			if err != io.EOF {
				log.Printf("failed to read from client: %v", err)
			}
			return
		}

		// Extract the length
		length := binary.BigEndian.Uint32(buf[:4])
		if length > 65535 {
			log.Printf("packet too large: %d", length)
			return
		}

		// Read the encrypted packet
		_, err = io.ReadFull(conn, buf[:length])
		if err != nil {
			log.Printf("failed to read encrypted packet from client: %v", err)
			return
		}

		// Retrieve the session for this client
		sessionValue, sessionExists := localIpToSession.Load(*extIpAddr)
		if !sessionExists {
			log.Printf("failed to load session for IP %s", *extIpAddr)
			continue
		}

		session := sessionValue.(*ChaCha20.Session)

		// Create AAD for decryption using C2SCounter
		aad := session.CreateAAD(false, session.C2SCounter)

		// Decrypt the data
		packet, err := session.Decrypt(buf[:length], aad)
		if err != nil {
			log.Printf("failed to decrypt packet: %v", err)
			return
		}

		// Increment the C2SCounter after successful decryption
		atomic.AddUint64(&session.C2SCounter, 1)

		// Validate the packet (optional but recommended)
		if _, err := packets.Parse(packet); err != nil {
			log.Printf("invalid IP packet structure: %v", err)
			continue
		}

		// Write the decrypted packet to the TUN interface
		if err := WriteToTun(tunFile, packet); err != nil {
			log.Printf("failed to write to TUN: %v", err)
			return
		}
	}
}

func createNewTun() error {
	conf, err := (&server.Conf{}).Read()
	if err != nil {
		return fmt.Errorf("failed to read server conf: %s", err)
	}

	_, _ = utils.DelTun(conf.IfName)

	name, err := UpNewTun(conf.IfName)
	if err != nil {
		log.Fatalf("failed to create interface %v: %v", conf.IfName, err)
	}
	fmt.Printf("Created TUN interface: %v\n", name)

	_, err = utils.AssignTunIP(conf.IfName, conf.IfIP)
	if err != nil {
		return err
	}
	fmt.Printf("assigned IP %s to interface %s\n", conf.TCPPort, conf.IfName)

	return nil
}

func getIfName(tunFile *os.File) (string, error) {
	var ifr ifreq

	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		tunFile.Fd(),
		uintptr(unix.TUNGETIFF),
		uintptr(unsafe.Pointer(&ifr)),
	)
	if errno != 0 {
		return "", errno
	}

	ifName := string(ifr.Name[:])
	ifName = strings.Trim(string(ifr.Name[:]), "\x00")
	return ifName, nil
}
