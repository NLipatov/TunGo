package network

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"etha-tunnel/handshake"
	"etha-tunnel/network/packets"
	"etha-tunnel/network/utils"
	"fmt"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
)

func Serve(tunFile *os.File, listenPort string) error {
	externalIfName, err := utils.GetDefaultIf()
	if err != nil {
		return err
	}

	err = enableNAT(externalIfName)
	if err != nil {
		return fmt.Errorf("failed enabling NAT: %v", err)
	}
	defer disableNAT(externalIfName)

	err = setupForwarding(tunFile, externalIfName)
	if err != nil {
		return fmt.Errorf("failed to set up forwarding: %v", err)
	}
	defer clearForwarding(tunFile, externalIfName)

	// Map to keep track of connected clients
	var localIpMap sync.Map
	var localIpToSessionMap sync.Map

	// Start a goroutine to read from TUN interface and send to clients
	go func() {
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
				session := sessionValue.(*handshake.Session)
				aad := session.CreateAAD(true, session.S2CCounter)
				encryptedPacket, err := session.Encrypt(packet, aad)
				if err != nil {
					log.Printf("failder to encrypt a package")
					continue
				}
				atomic.AddUint64(&session.S2CCounter, 1)
				///
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
	}()

	// Listen for incoming client connections
	listener, err := net.Listen("tcp", listenPort)
	if err != nil {
		return fmt.Errorf("failed to listen on port %s: %v", listenPort, err)
	}
	defer listener.Close()
	log.Printf("server listening on port %s", listenPort)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("failed to accept connection: %v", err)
			continue
		}
		go registerClient(conn, tunFile, &localIpMap, &localIpToSessionMap)
	}
}

func registerClient(conn net.Conn, tunFile *os.File, localIpToConn *sync.Map, localIpToServerSessionMap *sync.Map) {
	/*edPublic*/ _, edPrivate := "m+tjQmYAG8tYt8xSTry29Mrl9SInd9pvoIsSywzPzdU=", "ZuQO8SI3rxY/v1sJn9DtGQ2vRgz/DiPg545iFYmSWleb62NCZgAby1i3zFJOvLb0yuX1Iid32m+gixLLDM/N1Q=="

	buf := make([]byte, 39+2+32+32+32) // 39(max ip) + 2(length headers) + 32 (ed25519 pub key) + 32 (curve pub key)
	_, err := conn.Read(buf)
	if err != nil {
		log.Printf("Failed to read from client: %v", err)
		return
	}

	clientHello, err := (&handshake.ClientHello{}).Read(buf)
	if err != nil {
		_ = fmt.Errorf("failed to deserialize registration message")
		return
	}

	// Server hello
	var curvePrivate [32]byte
	_, _ = io.ReadFull(rand.Reader, curvePrivate[:])
	curvePublic, _ := curve25519.X25519(curvePrivate[:], curve25519.Basepoint)
	serverNonce := make([]byte, 32)
	_, _ = io.ReadFull(rand.Reader, serverNonce)
	serverDataToSign := append(append(curvePublic, serverNonce...), clientHello.ClientNonce...)
	privateEd, err := base64.StdEncoding.DecodeString(edPrivate)
	if err != nil {
		log.Fatalf("failed to decode private ed key: %s", err)
	}
	serverSignature := ed25519.Sign(privateEd, serverDataToSign)
	serverHello, err := (&handshake.ServerHello{}).Write(&serverSignature, &serverNonce, &curvePublic)
	if err != nil {
		log.Fatalf("failed to for server hello: %s", err)
	}

	//// serverHello
	_, err = conn.Write(*serverHello)
	clientSignatureBuf := make([]byte, 64)
	_, err = conn.Read(clientSignatureBuf)
	if err != nil {
		log.Printf("Failed to read client signature: %v", err)
		return
	}
	clientSignature, err := (&handshake.ClientSignature{}).Read(clientSignatureBuf)
	if err != nil {
		log.Fatalf("failed to read client signature: %s", err)
	}

	if !ed25519.Verify(clientHello.EdPublicKey, append(append(clientHello.CurvePublicKey, clientHello.ClientNonce...), serverNonce...), clientSignature.ClientSignature) {
		log.Fatal("client failed signature check")
	}

	sharedSecret, _ := curve25519.X25519(curvePrivate[:], clientHello.CurvePublicKey)
	salt := sha256.Sum256(append(serverNonce, clientHello.ClientNonce...))
	infoSC := []byte("server-to-client") // server-key info
	infoCS := []byte("client-to-server") // client-key info
	serverToClientHKDF := hkdf.New(sha256.New, sharedSecret, salt[:], infoSC)
	clientToServerHKDF := hkdf.New(sha256.New, sharedSecret, salt[:], infoCS)
	keySize := chacha20poly1305.KeySize
	serverToClientKey := make([]byte, keySize)
	_, _ = io.ReadFull(serverToClientHKDF, serverToClientKey)
	clientToServerKey := make([]byte, keySize)
	_, _ = io.ReadFull(clientToServerHKDF, clientToServerKey)

	serverSession, err := handshake.NewSession(serverToClientKey, clientToServerKey, true)
	if err != nil {
		log.Fatalf("failed to create server session: %s\n", err)
	}

	serverSession.SessionId = sha256.Sum256(append(sharedSecret, salt[:]...))

	localIpToConn.Store(clientHello.IpAddress, conn)
	localIpToServerSessionMap.Store(clientHello.IpAddress, serverSession)

	log.Printf("%s registered as %s", conn.RemoteAddr(), clientHello.IpAddress)
	handleClient(conn, tunFile, localIpToConn, localIpToServerSessionMap, clientHello)
}

func handleClient(conn net.Conn, tunFile *os.File, localIpToConn *sync.Map, localIpToSession *sync.Map, hello *handshake.ClientHello) {
	defer func() {
		localIpToConn.Delete(hello.IpAddress)
		localIpToSession.Delete(hello.IpAddress)
		conn.Close()
		log.Printf("client disconnected: %s", conn.RemoteAddr())
	}()

	buf := make([]byte, 65535)
	for {
		// Read the length of the encrypted packet (4 bytes)
		_, err := io.ReadFull(conn, buf[:4])
		if err != nil {
			if err != io.EOF {
				log.Printf("Failed to read from client: %v", err)
			}
			return
		}

		// Extract the length
		length := binary.BigEndian.Uint32(buf[:4])
		if length > 65535 {
			log.Printf("Packet too large: %d", length)
			return
		}

		// Read the encrypted packet
		_, err = io.ReadFull(conn, buf[:length])
		if err != nil {
			log.Printf("Failed to read encrypted packet from client: %v", err)
			return
		}

		// Retrieve the session for this client
		sessionValue, sessionExists := localIpToSession.Load(hello.IpAddress)
		if !sessionExists {
			log.Printf("Failed to load session for IP %s", hello.IpAddress)
			continue
		}

		session := sessionValue.(*handshake.Session)

		// Create AAD for decryption using C2SCounter
		aad := session.CreateAAD(false, session.C2SCounter)

		// Decrypt the data
		packet, err := session.Decrypt(buf[:length], aad)
		if err != nil {
			log.Printf("Failed to decrypt packet: %v", err)
			return
		}

		// Increment the C2SCounter after successful decryption
		atomic.AddUint64(&session.C2SCounter, 1)

		// Validate the packet (optional but recommended)
		if _, err := packets.Parse(packet); err != nil {
			log.Printf("Invalid IP packet structure: %v", err)
			continue
		}

		// Write the decrypted packet to the TUN interface
		if err := WriteToTun(tunFile, packet); err != nil {
			log.Printf("Failed to write to TUN: %v", err)
			return
		}
	}
}

func enableNAT(iface string) error {
	cmd := exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING", "-o", iface, "-j", "MASQUERADE")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Failed to enable NAT on %s: %v, output: %s", iface, err, output)
	}
	return nil
}

func disableNAT(iface string) error {
	cmd := exec.Command("iptables", "-t", "nat", "-D", "POSTROUTING", "-o", iface, "-j", "MASQUERADE")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Failed to disable NAT on %s: %v, output: %s", iface, err, output)
	}
	return nil
}

func setupForwarding(tunFile *os.File, extIface string) error {
	// Get the name of the TUN interface
	tunName := getTunInterfaceName(tunFile)
	if tunName == "" {
		return fmt.Errorf("Failed to get TUN interface name")
	}

	// Set up iptables rules
	cmd := exec.Command("iptables", "-A", "FORWARD", "-i", extIface, "-o", tunName, "-m", "state", "--state",
		"RELATED,ESTABLISHED", "-j", "ACCEPT")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Failed to set up forwarding rule for %s -> %s: %v, output: %s", extIface, tunName, err, output)
	}

	cmd = exec.Command("iptables", "-A", "FORWARD", "-i", tunName, "-o", extIface, "-j", "ACCEPT")
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set up forwarding rule for %s -> %s: %v, output: %s", tunName, extIface, err, output)
	}
	return nil
}

func clearForwarding(tunFile *os.File, extIface string) error {
	tunName := getTunInterfaceName(tunFile)
	if tunName == "" {
		return fmt.Errorf("Failed to get TUN interface name")
	}

	cmd := exec.Command("iptables", "-D", "FORWARD", "-i", extIface, "-o", tunName, "-m", "state", "--state",
		"RELATED,ESTABLISHED", "-j", "ACCEPT")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Failed to remove forwarding rule for %s -> %s: %v, output: %s", extIface, tunName, err, output)
	}

	cmd = exec.Command("iptables", "-D", "FORWARD", "-i", tunName, "-o", extIface, "-j", "ACCEPT")
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Failed to remove forwarding rule for %s -> %s: %v, output: %s", tunName, extIface, err, output)
	}
	return nil
}

func getTunInterfaceName(tunFile *os.File) string {
	// Since we know the interface name, we can return it directly.
	// Alternatively, you could retrieve it from the file descriptor if needed.
	return "ethatun0"
}
