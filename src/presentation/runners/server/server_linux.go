package server

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"log"
	"os"
	"tungo/infrastructure/routing/server_routing/factory"
	"tungo/settings/server_configuration"
)

func StartServer(ctx context.Context) {
	configurationManager := server_configuration.NewManager()
	conf, confErr := configurationManager.Configuration()
	if confErr != nil {
		log.Fatal(confErr)
	}

	err := ensureEd25519KeyPairCreated(conf, configurationManager)
	if err != nil {
		log.Fatalf("failed to generate ed25519 keys: %s", err)
	}

	tunFactory := factory.NewServerTunFactory()
	deps := NewDependencies(tunFactory, *conf)

	runner := NewRunner(deps)
	runner.Run(ctx)
}

func ensureEd25519KeyPairCreated(conf *server_configuration.Configuration, manager *server_configuration.Manager) error {
	// if keys are generated
	if len(conf.Ed25519PublicKey) > 0 && len(conf.Ed25519PrivateKey) > 0 {
		return nil
	}

	envPublic := os.Getenv("ED25519_PUBLIC_KEY")
	encPrivate := os.Getenv("ED25519_PRIVATE_KEY")

	var public ed25519.PublicKey
	var private ed25519.PrivateKey
	if envPublic != "" && encPrivate != "" {
		publicKey, err := base64.StdEncoding.DecodeString(envPublic)
		if err != nil {
			log.Fatalf("failed to decode ED25519_PUBLIC_KEY from env var: %s", err)
		}
		privateKey, err := base64.StdEncoding.DecodeString(encPrivate)
		if err != nil {
			log.Fatalf("failed to decode ED25519_PRIVATE_KEY from env var: %s", err)
		}

		public = publicKey
		private = privateKey
	} else {
		publicKey, privateKey, keyGenerationErr := ed25519.GenerateKey(rand.Reader)
		if keyGenerationErr != nil {
			log.Fatalf("failed to generate ed25519 key pair: %s", keyGenerationErr)
		}
		public = publicKey
		private = privateKey
	}

	return manager.InjectEdKeys(public, private)
}
