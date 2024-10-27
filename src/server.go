package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"etha-tunnel/network"
	"etha-tunnel/server/forwarding/routing"
	"etha-tunnel/settings"
	"etha-tunnel/settings/server"
	"log"
	"sync"
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

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		err = startTCPServer(conf.TCPSettings)
		if err != nil {
			log.Print(err)
		}

	}()

	go func() {
		defer wg.Done()
		err = startUDPServer(conf.UDPSettings)
		if err != nil {
			log.Print(err)
		}
	}()

	wg.Wait()
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

func startTCPServer(settings settings.ConnectionSettings) error {
	err := network.CreateNewTun(settings)
	if err != nil {
		log.Fatalf("failed to create TUN: %s", err)
	}

	tunFile, err := network.OpenTunByName(settings.InterfaceName)
	if err != nil {
		log.Fatalf("failed to open TUN interface: %v", err)
	}
	defer func() {
		_ = tunFile.Close()
	}()

	err = routing.StartTCPRouting(tunFile, settings.ConnectionPort)
	if err != nil {
		return err
	}

	return nil
}

func startUDPServer(settings settings.ConnectionSettings) error {
	err := network.CreateNewTun(settings)
	if err != nil {
		log.Fatalf("failed to create TUN: %s", err)
	}

	tunFile, err := network.OpenTunByName(settings.InterfaceName)
	if err != nil {
		log.Fatalf("failed to open TUN interface: %v", err)
	}
	defer func() {
		_ = tunFile.Close()
	}()

	err = routing.StartUDPRouting(tunFile, settings.ConnectionPort)
	if err != nil {
		return err
	}

	return nil
}
