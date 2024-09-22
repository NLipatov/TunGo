package main

import (
	"etha-tunnel/network"
	"etha-tunnel/network/utils"
	"etha-tunnel/settings/server"
	"fmt"
	"log"
)

func main() {
	conf, err := (&server.Conf{}).Read()
	if err != nil {
		log.Fatalf("failed to read configuration: %v", err)
	}

	err = configureServer(conf)
	tunFile, err := network.OpenTunByName(conf.IfName)
	if err != nil {
		log.Fatalf("Failed to open TUN interface: %v", err)
	}
	defer tunFile.Close()

	err = network.Serve(tunFile, conf.TCPPort)
	if err != nil {
		log.Print(err)
	}
}

func configureServer(conf *server.Conf) error {
	_, _ = utils.DelTun(conf.IfName)

	name, err := network.UpNewTun(conf.IfName)
	if err != nil {
		log.Fatalf("Failed to create interface %v: %v", conf.IfName, err)
	}
	fmt.Printf("Created TUN interface: %v\n", name)

	_, err = utils.AssignTunIP(conf.IfName, conf.IfIP)
	if err != nil {
		return err
	}
	fmt.Printf("Assigned IP %s to interface %s\n", conf.TCPPort, conf.IfName)

	return nil
}
