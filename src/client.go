package main

import (
	"encoding/binary"
	"etha-tunnel/handshake/client"
	"etha-tunnel/network"
	"etha-tunnel/network/utils"
	"etha-tunnel/settings/clientConfiguration"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
)

func main() {
	conf, err := (&clientConfiguration.Conf{}).Read()
	if err != nil {
		log.Fatalf("failed to read configuration: %v", err)
	}
	clientConfigurationErr := configureClient(conf)
	if clientConfigurationErr != nil {
		log.Fatalf("failed to configure client: %v", clientConfigurationErr)
	}

	tunFile, err := network.OpenTunByName(conf.IfName)
	if err != nil {
		log.Fatalf("Failed to open TUN interface: %v", err)
	}
	defer tunFile.Close()

	conn, err := net.Dial("tcp", conf.ServerTCPAddress)
	if err != nil {
		log.Fatalf("Failed to connect to server: %v", err)
	}
	defer conn.Close()
	log.Printf("Connected to server at %s", conf.ServerTCPAddress)

	err = register(conn, conf)
	if err != nil {
		log.Fatalf("registration failed: %s", err)
	}

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

func register(conn net.Conn, conf *clientConfiguration.Conf) error {
	cH, err := (&client.ClientHello{}).Write(4, strings.Split(conf.IfIP, "/")[0])
	if err != nil {
		return fmt.Errorf("failed to serialize registration message")
	}

	_, err = conn.Write(cH)
	if err != nil {
		return fmt.Errorf("failed to notice server on local address: %v", err)
	}

	//Mocked serverConfiguration hello
	sH := make([]byte, 32)
	_, err = conn.Read(sH)
	if err != nil {
		return fmt.Errorf("failed to read serverConfiguration-hello message")
	}

	log.Printf("registered at %v", conf.IfIP)

	return nil
}

func configureClient(conf *clientConfiguration.Conf) error {
	_, _ = utils.DelTun(conf.IfName)
	name, err := network.UpNewTun(conf.IfName)
	if err != nil {
		return fmt.Errorf("failed to create interface %v: %v", conf.IfName, err)
	}
	fmt.Printf("Created TUN interface: %v\n", name)

	_, err = utils.AssignTunIP(conf.IfName, conf.IfIP)
	if err != nil {
		return err
	}
	fmt.Printf("Assigned IP %s to interface %s\n", conf.IfIP, conf.IfName)

	_, err = utils.SetDefaultIf(conf.IfName)
	if err != nil {
		return err
	}
	fmt.Printf("Set %s as default gateway\n", conf.IfName)

	return nil
}
