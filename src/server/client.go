package main

import (
	"encoding/binary"
	"etha-tunnel/network"
	"fmt"
	"io"
	"log"
	"net"
)

const (
	serverAddr = "localhost:8080"
)

func main() {
	clientInterfaceName := "ethatun1"
	err := network.DeleteInterface(clientInterfaceName)
	name, err := network.UpNewTun(clientInterfaceName)
	if err != nil {
		log.Fatalf("Failed to create interface %v: %v", clientInterfaceName, err)
	}
	defer func() {
		err = network.DeleteInterface(clientInterfaceName)
		if err != nil {
			log.Fatalf("Failed to delete interface %v: %v", clientInterfaceName, err)
		}
		fmt.Printf("%s interface deleted\n", clientInterfaceName)
	}()

	tunFile, err := network.OpenTunByName(name)
	if err != nil {
		log.Fatalf("Failed to open TUN interface: %v", err)
	}
	defer tunFile.Close()

	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		log.Fatalf("Failed to connect to server: %v", err)
	}
	defer conn.Close()

	go func() {
		buf := make([]byte, 1500)
		for {
			_, err := io.ReadFull(conn, buf[:4])
			if err != nil {
				log.Fatalf("Failed to read from server: %v", err)
			}
			length := binary.BigEndian.Uint32(buf[:4])
			if length > 1500 {
				log.Fatalf("Packet too large: %d", length)
			}
			_, err = io.ReadFull(conn, buf[:length])
			if err != nil {
				log.Fatalf("Failed to read from server: %v", err)
			}
			_, err = tunFile.Write(buf[:length])
			if err != nil {
				log.Fatalf("Failed to write to TUN: %v", err)
			}
		}
	}()

	buf := make([]byte, 1500)
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
}
