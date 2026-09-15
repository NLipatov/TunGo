package main

import (
	"context"
	"fmt"
)

func runTunnel(ctx context.Context, server *server, protocol, directory, artifacts string) error {
	for _, f := range families {
		fmt.Printf("IPv%d tunnel: client %s -> server %s\n", f.number, f.client, f.server)
		fmt.Printf("IPv%d target network: server %s -> HTTP target %s\n", f.number, f.nat, f.target)
	}
	checksum, err := server.checkTarget(ctx)
	if err != nil {
		return err
	}
	fmt.Println("Generating client configuration and starting TunGo server")
	config, err := server.startServer(ctx, protocol)
	if err != nil {
		return err
	}
	if err := runRemoteClient(ctx, directory, artifacts, clientInput{Config: config, Checksum: checksum}); err != nil {
		return err
	}
	fmt.Println("Stopping TunGo server and checking interfaces, routes and firewall restoration")
	return server.stopServer(ctx)
}
