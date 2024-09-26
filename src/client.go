package main

import (
	"encoding/binary"
	"etha-tunnel/handshake/ChaCha20"
	"etha-tunnel/handshake/ChaCha20/handshakeHandlers"
	"etha-tunnel/network"
	"etha-tunnel/network/utils"
	"etha-tunnel/settings/client"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
)

func main() {
	conf, err := (&client.Conf{}).Read()
	if err != nil {
		log.Fatalf("failed to read configuration: %v", err)
	}
	clientConfigurationErr := configureClient(conf)
	if clientConfigurationErr != nil {
		log.Fatalf("failed to configure client: %v", clientConfigurationErr)
	}

	tunFile, err := network.OpenTunByName(conf.IfName)
	if err != nil {
		log.Fatalf("failed to open TUN interface: %v", err)
	}
	defer tunFile.Close()

	conn, err := net.Dial("tcp", conf.ServerTCPAddress)
	if err != nil {
		log.Fatalf("failed to connect to server: %v", err)
	}
	defer conn.Close()
	log.Printf("connected to server at %s", conf.ServerTCPAddress)

	session, err := handshakeHandlers.OnConnectedToServer(conn, conf)
	if err != nil {
		log.Fatalf("registration failed: %s", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// TUN -> TCP
	go func(conn net.Conn, tunFile *os.File, session ChaCha20.Session) {
		defer wg.Done()
		forwardTunToTCP(conn, tunFile, session)
	}(conn, tunFile, *session)

	// TCP -> TUN
	go func(conn net.Conn, tunFile *os.File, session ChaCha20.Session) {
		defer wg.Done()
		forwardTCPToTun(conn, tunFile, session)
	}(conn, tunFile, *session)

	wg.Wait()
}

func configureClient(conf *client.Conf) error {
	_, _ = utils.DelTun(conf.IfName)
	name, err := network.UpNewTun(conf.IfName)
	if err != nil {
		return fmt.Errorf("failed to create interface %v: %v", conf.IfName, err)
	}
	fmt.Printf("created TUN interface: %v\n", name)

	_, err = utils.AssignTunIP(conf.IfName, conf.IfIP)
	if err != nil {
		return err
	}
	fmt.Printf("assigned IP %s to interface %s\n", conf.IfIP, conf.IfName)

	serverIP, _, err := net.SplitHostPort(conf.ServerTCPAddress)
	if err != nil {
		return fmt.Errorf("failed to parse server address: %v", err)
	}

	cmd := exec.Command("ip", "route", "get", serverIP)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get route to server IP: %v", err)
	}

	routeInfo := string(output)
	var viaGateway, devInterface string
	fields := strings.Fields(routeInfo)
	for i, field := range fields {
		if field == "via" && i+1 < len(fields) {
			viaGateway = fields[i+1]
		}
		if field == "dev" && i+1 < len(fields) {
			devInterface = fields[i+1]
		}
	}
	if devInterface == "" {
		return fmt.Errorf("failed to parse route to server IP")
	}

	if viaGateway != "" {
		cmd = exec.Command("ip", "route", "add", serverIP, "via", viaGateway, "dev", devInterface)
	} else {
		cmd = exec.Command("ip", "route", "add", serverIP, "dev", devInterface)
	}
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to add route to server IP: %v", err)
	}

	fmt.Printf("added route to server %s via %s dev %s\n", serverIP, viaGateway, devInterface)

	_, err = utils.SetDefaultIf(conf.IfName)
	if err != nil {
		return err
	}
	fmt.Printf("set %s as default gateway\n", conf.IfName)

	return nil
}

func forwardTunToTCP(conn net.Conn, tunFile *os.File, session ChaCha20.Session) {
	buf := make([]byte, 65535)
	for {
		n, err := tunFile.Read(buf)
		if err != nil {
			log.Fatalf("failed to read from TUN: %v", err)
		}

		aad := session.CreateAAD(false, session.C2SCounter)

		encryptedPacket, err := session.Encrypt(buf[:n], aad)
		if err != nil {
			log.Fatalf("failed to encrypt packet: %v", err)
		}

		atomic.AddUint64(&session.C2SCounter, 1)

		length := uint32(len(encryptedPacket))
		lengthBuf := make([]byte, 4)
		binary.BigEndian.PutUint32(lengthBuf, length)
		_, err = conn.Write(append(lengthBuf, encryptedPacket...))
		if err != nil {
			log.Fatalf("failed to write to server: %v", err)
		}
	}
}

func forwardTCPToTun(conn net.Conn, tunFile *os.File, session ChaCha20.Session) {
	buf := make([]byte, 65535)
	for {
		_, err := io.ReadFull(conn, buf[:4])
		if err != nil {
			if err != io.EOF {
				log.Fatalf("failed to read from server: %v", err)
			}
			return
		}
		length := binary.BigEndian.Uint32(buf[:4])

		if length > 65535 {
			log.Fatalf("packet too large: %d", length)
			return
		}

		_, err = io.ReadFull(conn, buf[:length])
		if err != nil {
			log.Fatalf("failed to read encrypted packet: %v", err)
			return
		}

		aad := session.CreateAAD(true, session.S2CCounter)
		decrypted, err := session.Decrypt(buf[:length], aad)
		if err != nil {
			log.Fatalf("failed to decrypt server packet: %s\n", err)
		}

		atomic.AddUint64(&session.S2CCounter, 1)

		_, err = tunFile.Write(decrypted)
		if err != nil {
			log.Fatalf("failed to write to TUN: %v", err)
			return
		}
	}
}
