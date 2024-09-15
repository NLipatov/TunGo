package main

import (
	"etha-tunnel/network"
	"fmt"
	"log"
	"os/exec"
)

const (
	serverIfName = "ethatun0"
	tunIP        = "10.0.0.1/24" //ToDo: move to server configuration file
	listenPort   = ":8080"       //ToDo: move to server configuration file
)

func main() {
	err := configureServer()
	tunFile, err := network.OpenTunByName(serverIfName)
	if err != nil {
		log.Fatalf("Failed to open TUN interface: %v", err)
	}
	defer tunFile.Close()

	err = network.Serve(tunFile, listenPort)
	if err != nil {
		log.Print(err)
	}
}

func configureServer() error {
	_ = network.DeleteInterface(serverIfName)

	name, err := network.UpNewTun(serverIfName)
	if err != nil {
		log.Fatalf("Failed to create interface %v: %v", serverIfName, err)
	}
	fmt.Printf("Created TUN interface: %v\n", name)

	assignIP := exec.Command("ip", "addr", "add", tunIP, "dev", serverIfName)
	output, assignIPErr := assignIP.CombinedOutput()
	if assignIPErr != nil {
		log.Fatalf("Failed to assign IP to TUN %v: %v, output: %s", serverIfName, assignIPErr, output)
	}
	fmt.Printf("Assigned IP %s to interface %s\n", tunIP, serverIfName)

	return nil
}
