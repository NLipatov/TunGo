package routing

import (
	"etha-tunnel/server/forwarding/serveripconfiguration"
	"etha-tunnel/server/forwarding/servertcptunforward"
	"fmt"
	"os"
	"sync"
)

func Start(tunFile *os.File, listenPort string) error {
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
		servertcptunforward.ToTCP(tunFile, &extToLocalIp, &extIpToSession)
	}()

	// TCP -> TUN
	go func() {
		defer wg.Done()
		servertcptunforward.ToTun(listenPort, tunFile, &extToLocalIp, &extIpToSession)
	}()

	wg.Wait()
	return nil
}
