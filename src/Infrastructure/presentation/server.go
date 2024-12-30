package presentation

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"log"
	"sync"
	"tungo/Application/server/routing"
	"tungo/Domain/settings"
	"tungo/Domain/settings/server"
	"tungo/Infrastructure/cmd"
	"tungo/Infrastructure/network"
)

func StartServer() {
	conf, err := (&server.Conf{}).Read()
	if err != nil {
		log.Fatalf("failed to read configuration: %v", err)
	}

	err = ensureEd25519KeyPairCreated(conf)
	if err != nil {
		log.Fatalf("failed to generate ed25519 keys: %s", err)
	}

	var wg sync.WaitGroup
	if conf.EnableTCP {
		wg.Add(1)

		go func() {
			defer wg.Done()
			err = startTCPServer(conf.TCPSettings)
			if err != nil {
				log.Print(err)
			}

		}()
	}
	if conf.EnableUDP {
		wg.Add(1)

		go func() {
			defer wg.Done()
			err = startUDPServer(conf.UDPSettings)
			if err != nil {
				log.Print(err)
			}
		}()
	}

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
	tunFile, err := network.SetupServerTun(settings)
	if err != nil {
		log.Fatalf("failed to open TUN interface: %v", err)
	}
	defer func() {
		_ = tunFile.Close()
	}()

	// Create a context that can be canceled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start a goroutine to listen for user input
	go cmd.ListenForCommand(cancel, "server")

	// Setup server
	err = network.Configure(tunFile)
	if err != nil {
		return fmt.Errorf("failed to configure a server: %s\n", err)
	}
	defer network.Unconfigure(tunFile)

	err = routing.StartTCPRouting(ctx, tunFile, settings)
	if err != nil {
		return err
	}

	return nil
}

func startUDPServer(settings settings.ConnectionSettings) error {
	tunFile, err := network.SetupServerTun(settings)
	if err != nil {
		log.Fatalf("failed to open TUN interface: %v", err)
	}
	defer func() {
		_ = tunFile.Close()
	}()

	// Create a context that can be canceled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start a goroutine to listen for user input
	go cmd.ListenForCommand(cancel, "server")

	// Setup server
	err = network.Configure(tunFile)
	if err != nil {
		return fmt.Errorf("failed to configure a server: %s\n", err)
	}
	defer network.Unconfigure(tunFile)
	err = routing.StartUDPRouting(ctx, tunFile, settings)
	if err != nil {
		return err
	}

	return nil
}
