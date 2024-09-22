package network

import (
	"encoding/binary"
	"etha-tunnel/crypto/asymmetric/curve25519"
	"etha-tunnel/crypto/symmetric/chacha20"
	"etha-tunnel/handshake/client"
	"etha-tunnel/handshake/server"
	"etha-tunnel/network/packets"
	"etha-tunnel/network/utils"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"sync"
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
				length := uint32(len(packet))
				lengthBuf := make([]byte, 4)
				binary.BigEndian.PutUint32(lengthBuf, length)
				_, err := conn.Write(append(lengthBuf, packet...))
				if err != nil {
					log.Printf("failed to send packet to client: %v", err)
					localIpMap.Delete(destinationIP)
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
		go registerClient(conn, tunFile, &localIpMap)
	}
}

func registerClient(conn net.Conn, tunFile *os.File, localIpToConn *sync.Map) {
	buf := make([]byte, 73) //1 (IpVersion) + 1 (IpAddressLength) + 39 (IPv6 address) + 32 (PublicKey) = 73 bytes max
	_, err := conn.Read(buf)
	if err != nil {
		log.Printf("Failed to read from client: %v", err)
		return
	}

	cH, err := (&client.ClientHello{}).Read(buf)
	if err != nil {
		_ = fmt.Errorf("failed to deserialize registration message")
		return
	}

	//Server hello
	privateKey, publicKey, err := curve25519.GenerateCurve25519KeyPair()
	if err != nil {
		log.Fatalf("failed to generate curve25519 keypair: %s", err)
	}

	cc20Key, err := chacha20.GenerateKey()
	fmt.Printf("using cc20 key: %s\n", cc20Key)
	if err != nil {
		log.Fatalf("failed to generate ChaCha20 key: %s", err)
	}
	sH, err := (&server.ServerHello{}).Write(cc20Key, cH.PublicKey, privateKey, publicKey)
	if err != nil {
		log.Fatalf("failed to generate server hello: %s", err)
	}
	_, err = conn.Write(sH)

	localIpToConn.Store(cH.IpAddress, conn)

	log.Printf("%s registered as %s", conn.RemoteAddr(), cH.IpAddress)
	handleClient(conn, tunFile, localIpToConn, cH)
}

func handleClient(conn net.Conn, tunFile *os.File, localIpToConn *sync.Map, hello *client.ClientHello) {
	defer func() {
		localIpToConn.Delete(hello.IpVersion)
		conn.Close()
		log.Printf("client disconnected: %s", conn.RemoteAddr())
	}()

	buf := make([]byte, 65535)
	for {
		// Read packet length
		_, err := io.ReadFull(conn, buf[:4])
		if err != nil {
			if err != io.EOF {
				log.Printf("Failed to read from client: %v", err)
			}
			return
		}
		length := binary.BigEndian.Uint32(buf[:4])
		if length > 65535 {
			log.Printf("Packet too large: %d", length)
			return
		}
		// Read packet
		_, err = io.ReadFull(conn, buf[:length])
		if err != nil {
			log.Printf("Failed to read from client: %v", err)
			return
		}
		packet := buf[:length]

		// Write packet to TUN interface
		err = WriteToTun(tunFile, packet)
		if err != nil {
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
