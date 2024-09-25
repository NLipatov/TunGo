package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"etha-tunnel/clientConfGenerator"
	"etha-tunnel/network"
	"etha-tunnel/network/utils"
	"etha-tunnel/settings/server"
	"fmt"
	"log"
	"os"
)

func main() {
	conf, err := (&server.Conf{}).Read()
	if err != nil {
		log.Fatalf("failed to read configuration: %v", err)
	}

	if len(conf.Ed25519PublicKey) == 0 || len(conf.Ed25519PrivateKey) == 0 {
		edPub, ed, keyGenerationErr := ed25519.GenerateKey(rand.Reader)
		if keyGenerationErr != nil {
			log.Fatalf("failed to generate ed25519 key pair: %s", err)
		}
		err = conf.InsertEdKeys(edPub, ed)
		if err != nil {
			log.Fatalf("failed to insert ed25519 keys to server conf: %s", err)
		}
	}

	// Handle args
	args := os.Args
	if len(args[1:]) == 1 && args[1] == "gen" {
		newConf, err := clientConfGenerator.Generate()
		if err != nil {
			log.Fatalf("failed to generate client conf: %s\n", err)
		}

		marshalled, err := json.Marshal(newConf)
		if err != nil {
			log.Fatalf("failed to marshalize client conf: %s\n", err)
		}

		fmt.Printf(string(marshalled))
		return
	}

	err = createNewTun(conf)
	tunFile, err := network.OpenTunByName(conf.IfName)
	if err != nil {
		log.Fatalf("failed to open TUN interface: %v", err)
	}
	defer tunFile.Close()

	err = network.Serve(tunFile, conf.TCPPort)
	if err != nil {
		log.Print(err)
	}
}

func createNewTun(conf *server.Conf) error {
	_, _ = utils.DelTun(conf.IfName)

	name, err := network.UpNewTun(conf.IfName)
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
