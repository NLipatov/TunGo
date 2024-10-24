package routing

import (
	"context"
	"etha-tunnel/inputcommands"
	"etha-tunnel/server/forwarding/serveripconfiguration"
	"etha-tunnel/server/forwarding/servertcptunforward"
	"fmt"
	"os"
	"sync"
)

func StartTCPRouting(tunFile *os.File, listenPort string) error {
	// Create a context that can be canceled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start a goroutine to listen for user input
	go inputcommands.ListenForCommand(cancel)

	// Setup server
	err := serveripconfiguration.Configure(tunFile)
	if err != nil {
		return fmt.Errorf("failed to configure a server: %s\n", err)
	}
	defer serveripconfiguration.Unconfigure(tunFile)

	// Map to keep track of connected clients
	var extToLocalIp sync.Map   // external ip to local ip map
	var extIpToSession sync.Map // external ip to session map

	var wg sync.WaitGroup
	wg.Add(2)

	// TUN -> TCP
	go func() {
		defer wg.Done()
		servertcptunforward.ToTCP(tunFile, &extToLocalIp, &extIpToSession, ctx)
	}()

	// TCP -> TUN
	go func() {
		defer wg.Done()
		servertcptunforward.ToTun(listenPort, tunFile, &extToLocalIp, &extIpToSession, ctx)
	}()

	wg.Wait()
	return nil
}
