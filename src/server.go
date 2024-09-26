package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"etha-tunnel/clientConfGenerator"
	"etha-tunnel/network"
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

	err = ensureEd25519KeyPairCreated(conf)
	if err != nil {
		log.Fatalf("failed to generate ed25519 keys: %s", err)
	}

	// Handle args
	args := os.Args
	if len(args[1:]) == 1 && args[1] == "gen" {
		newConf, err := clientConfGenerator.Generate()
		if err != nil {
			log.Fatalf("failed to generate client conf: %s\n", err)
		}

		marshalled, err := json.MarshalIndent(newConf, "", "  ")
		if err != nil {
			log.Fatalf("failed to marshalize client conf: %s\n", err)
		}

		fmt.Println(string(marshalled))
		return
	}

	err = network.CreateNewTun(conf)
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

func ensureEd25519KeyPairCreated(conf *server.Conf) error {
	// if keys are generated
	if len(conf.Ed25519PublicKey) > 0 && len(conf.Ed25519PrivateKey) > 0 {
		return nil
	}

	edPub, ed, keyGenerationErr := ed25519.GenerateKey(rand.Reader)
	if keyGenerationErr != nil {
		log.Fatalf("failed to generate ed25519 key pair: %s", keyGenerationErr)
	}
	err := conf.InsertEdKeys(edPub, ed)
	if err != nil {
		log.Fatalf("failed to insert ed25519 keys to server conf: %s", err)
	}

	return nil
}
