package main

import (
	"context"
	"log/slog"
)

func runTunnel(ctx context.Context, server *server, protocol, directory, logs string) error {
	for _, f := range families {
		slog.Info("Tunnel addresses", "family", f.number, "client", f.client, "server", f.server)
		slog.Info("Target network addresses", "family", f.number, "server", f.nat, "target", f.target)
	}
	checksum, err := server.checkTarget(ctx)
	if err != nil {
		return err
	}
	slog.Info("Generating client configuration and starting TunGo server")
	config, err := server.startServer(ctx, protocol)
	if err != nil {
		return err
	}
	if err := runRemoteClient(ctx, directory, logs, clientInput{Config: config, Checksum: checksum}); err != nil {
		return err
	}
	slog.Info("Stopping TunGo server and checking interfaces, routes and firewall restoration")
	return server.stopServer(ctx)
}
