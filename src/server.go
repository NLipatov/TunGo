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

	if len(conf.Ed25519PublicKey) == 0 || len(conf.Ed25519PrivateKey) == 0 {
		generateEd25519KeyPair(conf)
	}

	// Handle args
	args := os.Args
	if len(args[1:]) == 1 && args[1] == "gen" {
		generateClientConfiguration()
		return
	}

	// Start server
	tunFile, err := network.OpenTunByName(conf.IfName)
	if err != nil {
		log.Fatalf("failed to open TUN interface: %v", err)
	}
	defer tunFile.Close()

	err = network.ServeConnections(tunFile, conf.TCPPort)
	if err != nil {
		log.Print(err)
	}
}

func generateEd25519KeyPair(conf *server.Conf) {
	edPub, ed, keyGenerationErr := ed25519.GenerateKey(rand.Reader)
	if keyGenerationErr != nil {
		log.Fatalf("failed to generate ed25519 key pair: %s", keyGenerationErr)
	}
	insertKeysErr := conf.InsertEdKeys(edPub, ed)
	if insertKeysErr != nil {
		log.Fatalf("failed to insert ed25519 keys to server conf: %s", insertKeysErr)
	}
}

func generateClientConfiguration() {
	newConf, err := clientConfGenerator.Generate()
	if err != nil {
		log.Fatalf("failed to generate client conf: %s\n", err)
	}

	marshalled, err := json.MarshalIndent(newConf, "", "  ")
	if err != nil {
		log.Fatalf("failed to marshalize client conf: %s\n", err)
	}

	fmt.Println(string(marshalled))
}
