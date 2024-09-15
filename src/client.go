package main

import (
	"encoding/binary"
	"etha-tunnel/network"
	"fmt"
	"io"
	"log"
	"net"
	"os/exec"
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
	_ = network.DeleteInterface(clientIfName)
	name, err := network.UpNewTun(clientIfName)
	if err != nil {
		return fmt.Errorf("failed to create interface %v: %v", clientIfName, err)
	}
	fmt.Printf("Created TUN interface: %v\n", name)

	assignIP := exec.Command("ip", "addr", "add", clientTunIP, "dev", clientIfName)
	output, assignIPErr := assignIP.CombinedOutput()
	if assignIPErr != nil {
		return fmt.Errorf("failed to assign IP to TUN %v: %v, output: %s", clientIfName, assignIPErr, output)
	}
	fmt.Printf("Assigned IP %s to interface %s\n", clientTunIP, clientIfName)

	setAsDefaultGateway := exec.Command("ip", "route", "add", "default", "dev", clientIfName)
	output, setAsDefaultGatewayErr := setAsDefaultGateway.CombinedOutput()
	if setAsDefaultGatewayErr != nil {
		return fmt.Errorf("failed to set TUN as default gateway %v: %v, output: %s", clientIfName, setAsDefaultGatewayErr, output)
	}
	fmt.Printf("Set %s as default gateway\n", clientIfName)

	return nil
}
