package server

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"log"
	"os"
	"sync"
	"tungo/infrastructure/PAL/pal_factory"
	"tungo/infrastructure/routing/server_routing/factory"
	"tungo/presentation/interactive_commands"
	"tungo/settings"
	"tungo/settings/server_configuration"
)

type Runner struct {
	deps AppDependencies
}

func NewRunner(deps AppDependencies) *Runner {
	return &Runner{
		deps: deps,
	}
}

func (r *Runner) Run(ctx context.Context) {
	configurationManager := server_configuration.NewManager()
	conf, confErr := configurationManager.Configuration()
	if confErr != nil {
		log.Fatal(confErr)
	}
	err := r.ensureEd25519KeyPairCreated(conf, configurationManager)
	if err != nil {
		log.Fatalf("failed to generate ed25519 keys: %s", err)
	}

	// ToDo: move conf gen to bubble tea and cli
	go interactive_commands.ListenForCommand()

	var wg sync.WaitGroup
	if r.deps.Configuration().EnableTCP {
		wg.Add(1)

		connSettings := r.deps.Configuration().TCPSettings
		if err := r.deps.TunManager().DisposeTunDevices(connSettings); err != nil {
			log.Printf("error disposing tun devices: %s", err)
		}

		go func() {
			defer wg.Done()
			routeErr := r.route(ctx, connSettings)
			if routeErr != nil {
				log.Println(routeErr)
			}
		}()
	}

	if r.deps.Configuration().EnableUDP {
		wg.Add(1)

		connSettings := r.deps.Configuration().UDPSettings
		if err := r.deps.TunManager().DisposeTunDevices(connSettings); err != nil {
			log.Printf("error disposing tun devices: %s", err)
		}

		go func() {
			defer wg.Done()
			routeErr := r.route(ctx, connSettings)
			if routeErr != nil {
				log.Println(routeErr)
			}
		}()
	}

	wg.Wait()
}

func (r *Runner) route(ctx context.Context, settings settings.ConnectionSettings) error {
	workerFactory := pal_factory.NewServerWorkerFactory(settings)
	tunFactory := pal_factory.NewServerTunFactory()
	routerFactory := factory.NewServerRouterFactory()

	tun, tunErr := tunFactory.CreateTunDevice(settings)
	if tunErr != nil {
		log.Fatalf("error creating tun device: %s", tunErr)
	}

	worker, workerErr := workerFactory.CreateWorker(ctx, tun)
	if workerErr != nil {
		log.Fatalf("error creating worker: %s", workerErr)
	}

	router := routerFactory.CreateRouter(worker)

	routingErr := router.RouteTraffic(ctx)
	if routingErr != nil {
		log.Fatalf("error routing traffic: %s", routingErr)
	}

	return nil
}

func (r *Runner) ensureEd25519KeyPairCreated(conf *server_configuration.Configuration, manager *server_configuration.Manager) error {
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
