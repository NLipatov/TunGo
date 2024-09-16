package main

import (
	"encoding/binary"
	"etha-tunnel/network"
	"etha-tunnel/network/utils"
	"fmt"
	"io"
	"log"
	"net"
)

const (
	clientIfName = "ethatun0"
	clientTunIP  = "10.0.0.2/24"          //ToDo: move to client configuration file
	serverAddr   = "192.168.122.194:8080" //ToDo: move to client configuration file
)

func main() {
	clientConfigurationErr := configureClient()
	if clientConfigurationErr != nil {
		log.Fatalf("failed to configure client: %v", clientConfigurationErr)
	}

	tunFile, err := network.OpenTunByName(clientIfName)
	if err != nil {
		log.Fatalf("Failed to open TUN interface: %v", err)
	}
	defer tunFile.Close()

	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		log.Fatalf("Failed to connect to server: %v", err)
	}
	defer conn.Close()
	log.Printf("Connected to server at %s", serverAddr)

	go func() {
		buf := make([]byte, 65535)
		for {
			n, err := tunFile.Read(buf[4:])
			if err != nil {
				log.Fatalf("Failed to read from TUN: %v", err)
			}
			binary.BigEndian.PutUint32(buf[:4], uint32(n))
			_, err = conn.Write(buf[:4+n])
			if err != nil {
				log.Fatalf("Failed to write to server: %v", err)
			}
		}
	}()

	// Read from server and write to TUN
	buf := make([]byte, 65535)
	for {
		// Read packet length
		_, err := io.ReadFull(conn, buf[:4])
		if err != nil {
			if err != io.EOF {
				log.Fatalf("Failed to read from server: %v", err)
			}
			return
		}
		length := binary.BigEndian.Uint32(buf[:4])
		if length > 65535 {
			log.Fatalf("Packet too large: %d", length)
			return
		}
		// Read packet
		_, err = io.ReadFull(conn, buf[:length])
		if err != nil {
			log.Fatalf("Failed to read from server: %v", err)
			return
		}
		// Write packet to TUN interface
		_, err = tunFile.Write(buf[:length])
		if err != nil {
			log.Fatalf("Failed to write to TUN: %v", err)
			return
		}
	}
}

func configureClient() error {
	_, _ = utils.DelTun(clientIfName)
	name, err := network.UpNewTun(clientIfName)
	if err != nil {
		return fmt.Errorf("failed to create interface %v: %v", clientIfName, err)
	}
	fmt.Printf("Created TUN interface: %v\n", name)

	_, err = utils.AssignTunIP(clientIfName, clientTunIP)
	if err != nil {
		return err
	}
	fmt.Printf("Assigned IP %s to interface %s\n", clientTunIP, clientIfName)

	_, err = utils.SetDefaultIf(clientIfName)
	if err != nil {
		return err
	}
	fmt.Printf("Set %s as default gateway\n", clientIfName)

	return nil
}
